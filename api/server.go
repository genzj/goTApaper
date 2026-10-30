// Package api serves the service layer over HTTP on the loopback interface.
//
// It exists so the CLI, the test suite and eventually a desktop GUI all drive
// one implementation. It is not a public API: it binds to 127.0.0.1 only and
// requires a bearer token.
package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/api/wire"
	"github.com/genzj/goTApaper/service"
)

// LoopbackHost is the only address the API ever binds to.
//
// Never make this configurable. The API rewrites the user's configuration and
// can be made to issue outbound requests, so it must not be reachable from off
// the host.
const LoopbackHost = "127.0.0.1"

// Server is a running API server.
type Server struct {
	http     *http.Server
	listener net.Listener
	appDir   string
	token    string
	// handlers is retained so the embedding application can reach the event
	// hub to publish, and mint a browser session ticket, without going through
	// a socket to talk to itself.
	handlers *handlers
	// tickets issues the single-use tickets that hand a browser its session.
	tickets *ticketStore
}

// Options configures a Server.
type Options struct {
	// AppDir is where the token and endpoint files are written.
	AppDir string
	// Port to listen on; zero picks an ephemeral one.
	Port int
	// Config, Wallpaper, Downloader and Logs are the services to expose.
	Config     *service.Config
	Wallpaper  *service.Wallpaper
	Downloader *service.Downloader
	Logs       *service.Logs
	// Version is reported by GET /status.
	Version string
	// Lifecycle serves POST /daemon/restart and /daemon/stop. Nil answers
	// them with 501.
	Lifecycle Lifecycle
}

// Start begins serving in the background.
func Start(opts Options) (*Server, error) {
	token, err := loadOrCreateToken(opts.AppDir)
	if err != nil {
		return nil, err
	}

	listener, err := listen(opts.AppDir, opts.Port)
	if err != nil {
		return nil, err
	}

	logs := opts.Logs
	if logs == nil {
		// The log service holds no state of its own -- it reads the
		// process-wide hook -- so a caller that did not ask for one still gets
		// working log routes rather than a nil dereference.
		logs = service.NewLogs()
	}
	h := &handlers{
		config:     opts.Config,
		wallpaper:  opts.Wallpaper,
		downloader: opts.Downloader,
		logs:       logs,
		events:     newEventHub(),
		version:    opts.Version,
		lifecycle:  opts.Lifecycle,
	}
	if h.downloader != nil {
		// Subscribed here rather than by the caller, so every download --
		// whoever started it -- reaches the stream without the application
		// having to remember to forward it.
		h.downloader.OnChange(h.publishDownload)
	}

	tickets := newTicketStore()
	srv := &Server{
		http: &http.Server{
			Handler:           h.routes(token, tickets),
			ReadHeaderTimeout: 10 * time.Second,
		},
		listener: listener,
		appDir:   opts.AppDir,
		token:    token,
		handlers: h,
		tickets:  tickets,
	}

	if err := srv.writeEndpointFile(); err != nil {
		listener.Close()
		return nil, err
	}

	logrus.WithField("address", listener.Addr().String()).Info("API server listening")

	go func() {
		if err := srv.http.Serve(listener); err != nil && err != http.ErrServerClosed {
			logrus.WithError(err).Error("API server stopped unexpectedly")
		}
	}()

	return srv, nil
}

// listen binds the API to loopback.
//
// An explicit port is used as given. With port 0 the server would land on a
// new ephemeral port every start, which breaks the interface: its saved token
// lives in the browser's local storage, which is scoped to the origin, port
// included, and a tab left open across a restart points at a port nothing
// listens on any more. So port 0 first asks for the port the previous start
// got, and only falls back to a fresh one when that is taken. The bind address
// is LoopbackHost either way.
func listen(appDir string, port int) (net.Listener, error) {
	if port == 0 {
		if last := rememberedPort(appDir); last > 0 {
			addr := net.JoinHostPort(LoopbackHost, strconv.Itoa(last))
			if l, err := net.Listen("tcp", addr); err == nil {
				return l, nil
			}
			logrus.WithField("port", last).Debug(
				"the previously used API port is taken, picking another",
			)
		}
	}

	addr := net.JoinHostPort(LoopbackHost, strconv.Itoa(port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", addr, err)
	}

	if port == 0 {
		rememberPort(appDir, l.Addr().(*net.TCPAddr).Port)
	}
	return l, nil
}

// rememberedPort reads the port the last ephemeral bind got, or 0.
func rememberedPort(appDir string) int {
	data, err := os.ReadFile(wire.PortPath(appDir))
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

// rememberPort records an ephemeral port for the next start. A failure costs
// only that preference, so it is logged rather than returned.
func rememberPort(appDir string, port int) {
	if err := os.WriteFile(wire.PortPath(appDir), []byte(strconv.Itoa(port)+"\n"), 0600); err != nil {
		logrus.WithError(err).Debug("cannot remember the API port")
	}
}

// Addr reports the address being served.
func (s *Server) Addr() string { return s.listener.Addr().String() }

// writeEndpointFile records the listening address so the CLI can discover a
// running daemon without being told the port.
func (s *Server) writeEndpointFile() error {
	path := EndpointPath(s.appDir)
	if err := os.WriteFile(path, []byte(s.Addr()+"\n"), 0600); err != nil {
		return fmt.Errorf("cannot write the API endpoint file: %w", err)
	}
	return nil
}

// Shutdown stops the server and removes the endpoint file, so a stale address
// is not left behind for the CLI to try.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if rmErr := os.Remove(EndpointPath(s.appDir)); rmErr != nil && !os.IsNotExist(rmErr) {
		logrus.WithError(rmErr).Debug("cannot remove the API endpoint file")
	}
	return err
}

// routes builds the mux. Every route except the health check sits behind the
// token check.
func (h *handlers) routes(token string, tickets *ticketStore) http.Handler {
	mux := http.NewServeMux()

	// Unauthenticated: a liveness probe reveals nothing and is useful to a
	// client deciding whether to start a daemon.
	mux.HandleFunc("GET "+wire.BasePath+"/healthz", h.health)

	// Also unauthenticated, because it is how the interface acquires its
	// credential: a single-use ticket is traded for the token. See session.go.
	mux.Handle("POST "+ticketExchangePath, &ticketExchange{tickets: tickets, token: token})

	// The interface's static assets. Unauthenticated: the bundle holds no
	// secrets -- it asks for the token, and every API call it makes carries
	// it. Serving it openly is what lets the page load far enough to ask.
	mux.Handle("GET "+uiPath, &uiHandler{assets: uiAssets})
	// Bare "/ui" redirects, or the relative asset paths in index.html would
	// resolve against "/" and 404.
	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, uiPath, http.StatusMovedPermanently)
	})

	authed := http.NewServeMux()
	r := &router{mux: authed, allowed: map[string][]string{}}

	h.registerSettingRoutes(r)
	h.registerChannelRoutes(r)
	h.registerWatermarkRoutes(r)
	h.registerHistoryRoutes(r)
	h.registerWallpaperRoutes(r)
	h.registerLogRoutes(r)
	h.registerEventRoutes(r)
	h.registerDaemonRoutes(r)

	// Catch-alls, so an unrouted path or an unsupported method produces the
	// same JSON error shape as everything else. Go's ServeMux would otherwise
	// answer with a plain-text body that a client decoding JSON cannot read.
	//
	// These also shadow the mux's own 405 handling -- a pattern that matches
	// the path but not the method no longer matches at all once a prefix
	// pattern exists -- so the router reconstructs the distinction itself.
	authed.HandleFunc("/", r.unmatched)
	authed.HandleFunc(wire.BasePath+"/", r.unmatched)

	mux.Handle("/", requireToken(token, authed))

	// Recovery is outermost so a panic in any handler, or in the auth
	// middleware, still yields a JSON response instead of a dropped
	// connection.
	return recoverPanics(logRequests(requireLoopbackHost(requireLoopbackOrigin(mux))))
}

// logRequests records each request at debug level. Authorization headers are
// never logged, and a ticket cannot appear here: it travels in a URL fragment,
// which a browser never sends.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.WithField("method", r.Method).WithField(
			"path", r.URL.RequestURI(),
		).Debug("API request")
		next.ServeHTTP(w, r)
	})
}

// recoverPanics turns a panicking handler into a 500 with a JSON body.
//
// net/http would otherwise close the connection without a response, which a
// client sees as an unexplained transport error. The panic value is logged
// with its stack but not sent to the client, since it can carry internals.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// ErrAbortHandler is net/http's documented way of abandoning a
			// response on purpose; passing it on preserves that meaning.
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}

			logrus.WithFields(logrus.Fields{
				"method": r.Method,
				"path":   r.URL.Path,
				"panic":  recovered,
				"stack":  string(debug.Stack()),
			}).Error("a panic escaped an API handler")

			writeError(w, http.StatusInternalServerError, "internal error")
		}()

		next.ServeHTTP(w, r)
	})
}
