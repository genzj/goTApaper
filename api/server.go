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
}

// Options configures a Server.
type Options struct {
	// AppDir is where the token and endpoint files are written.
	AppDir string
	// Port to listen on; zero picks an ephemeral one.
	Port int
	// Config and Wallpaper are the services to expose.
	Config    *service.Config
	Wallpaper *service.Wallpaper
}

// Start begins serving in the background.
func Start(opts Options) (*Server, error) {
	token, err := loadOrCreateToken(opts.AppDir)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(LoopbackHost, fmt.Sprintf("%d", opts.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", addr, err)
	}

	h := &handlers{config: opts.Config, wallpaper: opts.Wallpaper}

	srv := &Server{
		http: &http.Server{
			Handler:           h.routes(token),
			ReadHeaderTimeout: 10 * time.Second,
		},
		listener: listener,
		appDir:   opts.AppDir,
		token:    token,
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
func (h *handlers) routes(token string) http.Handler {
	mux := http.NewServeMux()

	// Unauthenticated: a liveness probe reveals nothing and is useful to a
	// client deciding whether to start a daemon.
	mux.HandleFunc("GET "+wire.BasePath+"/healthz", h.health)

	authed := http.NewServeMux()
	r := &router{mux: authed, allowed: map[string][]string{}}

	r.handle("GET", "/settings", h.listSettings)
	r.handle("GET", "/settings/{key}", h.getSetting)
	r.handle("PUT", "/settings/{key}", h.putSetting)
	r.handle("DELETE", "/settings/{key}", h.deleteSetting)
	r.handle("GET", "/setting-definitions", h.settingDefinitions)

	r.handle("GET", "/channels", h.listChannels)
	r.handle("POST", "/channels", h.upsertChannel)
	r.handle("POST", "/channels/reorder", h.reorderChannels)
	r.handle("GET", "/channels/{name}", h.getChannel)
	r.handle("PUT", "/channels/{name}", h.upsertChannel)
	r.handle("DELETE", "/channels/{name}", h.deleteChannel)
	r.handle("GET", "/channel-types", h.channelTypes)

	r.handle("GET", "/channel-templates", h.listTemplates)
	r.handle("POST", "/channel-templates", h.upsertTemplate)
	r.handle("GET", "/channel-templates/{name}", h.getTemplate)
	r.handle("PUT", "/channel-templates/{name}", h.upsertTemplate)
	r.handle("DELETE", "/channel-templates/{name}", h.deleteTemplate)
	r.handle("POST", "/channel-templates/{name}/inline", h.inlineTemplate)

	r.handle("GET", "/watermark-layers", h.listWatermarkLayers)
	r.handle("POST", "/watermark-layers", h.upsertWatermarkLayer)
	r.handle("PUT", "/watermark-layers/{id}", h.upsertWatermarkLayer)
	r.handle("DELETE", "/watermark-layers/{id}", h.deleteWatermarkLayer)

	r.handle("GET", "/history", h.listHistory)
	r.handle("POST", "/refresh", h.refresh)

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
	return recoverPanics(logRequests(mux))
}

// logRequests records each request at debug level. Authorization headers are
// never logged.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.WithField("method", r.Method).WithField("path", r.URL.Path).Debug("API request")
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
