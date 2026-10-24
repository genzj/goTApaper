package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Where the browser-facing routes live. Outside wire.BasePath, because they
// serve a user interface rather than the JSON API.
const (
	uiPath        = "/ui/"
	uiSessionPath = "/ui/session"
)

const (
	// SessionCookieName carries the API token for a browser.
	SessionCookieName = "gotapaper_session"

	// ticketTTL is how long a tray-minted ticket stays valid. Long enough for
	// a browser to cold-start, short enough that one left in a shell history
	// or a crash log is useless.
	ticketTTL = 60 * time.Second

	// ticketBytes is the entropy in a ticket before encoding.
	ticketBytes = 32
)

// ticketStore issues the single-use tickets that hand a browser its session.
//
// A ticket exists because the alternatives are both worse. Putting the API
// token in the URL writes a long-lived credential into browser history, the
// Referer header and any shell the URL was pasted into. Serving the UI without
// authentication at all would leave the user's provider API keys readable by
// every local process. A ticket is a one-shot, short-lived stand-in: it is
// exchanged for an HttpOnly cookie and immediately destroyed, so the URL that
// leaks is already worthless.
type ticketStore struct {
	mu      sync.Mutex
	tickets map[string]time.Time
	now     func() time.Time
}

func newTicketStore() *ticketStore {
	return &ticketStore{tickets: map[string]time.Time{}, now: time.Now}
}

// Mint returns a fresh ticket.
func (t *ticketStore) Mint() (string, error) {
	raw := make([]byte, ticketBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	// URL-safe, since this travels as a query parameter.
	ticket := base64.RawURLEncoding.EncodeToString(raw)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.expireLocked()
	t.tickets[ticket] = t.now().Add(ticketTTL)
	return ticket, nil
}

// Redeem consumes a ticket, reporting whether it was valid. A ticket is
// removed whether or not it had expired, so it can never be presented twice.
func (t *ticketStore) Redeem(ticket string) bool {
	if ticket == "" {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Found by a constant-time comparison across the (tiny) set rather than by
	// a map lookup: a map hit on a secret leaks its prefix through timing.
	var deadline time.Time
	var found bool
	for candidate, expiry := range t.tickets {
		if len(candidate) == len(ticket) &&
			subtle.ConstantTimeCompare([]byte(candidate), []byte(ticket)) == 1 {
			deadline = expiry
			found = true
			delete(t.tickets, candidate)
			break
		}
	}
	if !found {
		return false
	}
	return t.now().Before(deadline)
}

// expireLocked drops tickets past their deadline, so an application left
// running does not accumulate them.
func (t *ticketStore) expireLocked() {
	now := t.now()
	for ticket, deadline := range t.tickets {
		if !now.Before(deadline) {
			delete(t.tickets, ticket)
		}
	}
}

// sessionHandler exchanges a ticket for a session cookie.
type sessionHandler struct {
	tickets *ticketStore
	token   string
}

func (s *sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.tickets.Redeem(r.URL.Query().Get("ticket")) {
		// Logged at warning for the same reason a rejected token is: this is
		// the user's only signal that something local is probing an interface
		// that can read their API keys.
		logrus.WithFields(logrus.Fields{
			"path":   r.URL.Path,
			"remote": r.RemoteAddr,
		}).Warn("rejected a session request with no valid ticket")

		writeError(w, http.StatusForbidden,
			"this link is invalid or has expired; open the interface from the tray menu again")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  SessionCookieName,
		Value: s.token,
		Path:  "/",
		// HttpOnly so a scripting bug in the UI cannot read the token;
		// SameSite=Strict so another page in the same browser cannot drive the
		// API with it. Secure is deliberately not set: this is plain HTTP on
		// loopback, and setting it would stop the cookie being stored at all.
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})

	// Redirect to a clean URL so the ticket does not stay in the address bar
	// or in history, even though it has already been spent.
	http.Redirect(w, r, uiPath, http.StatusFound)
}

// SessionURL mints a ticket and returns the URL that opens the interface in a
// browser with a session already established.
func (s *Server) SessionURL() (string, error) {
	ticket, err := s.tickets.Mint()
	if err != nil {
		return "", err
	}
	return "http://" + s.Addr() + uiSessionPath + "?ticket=" + url.QueryEscape(ticket), nil
}

// UIURL is the interface's address for a client that already holds the token,
// which is the case inside the native webview.
func (s *Server) UIURL() string {
	return "http://" + s.Addr() + uiPath
}

// isLoopbackOrigin reports whether an Origin header belongs to this
// application rather than to some other page in the user's browser.
func isLoopbackOrigin(origin string) bool {
	if origin == "" {
		// Absent on same-origin navigations and on non-browser clients such as
		// curl and the CLI, neither of which can be driven by a hostile page.
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == LoopbackHost || host == "localhost" || host == "::1"
}

// requireLoopbackOrigin rejects a cross-origin state-changing request.
//
// SameSite=Strict already stops a hostile page sending the session cookie, so
// this is a second, explicit check rather than the only one: a browser that
// mishandles SameSite, or a future route reached with the bearer token from a
// page, still cannot drive a write.
func requireLoopbackOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			// Safe methods. The API serves no state-changing GET.
		default:
			if !isLoopbackOrigin(r.Header.Get("Origin")) {
				logrus.WithFields(logrus.Fields{
					"method": r.Method,
					"path":   r.URL.Path,
					"origin": r.Header.Get("Origin"),
				}).Warn("rejected a cross-origin API request")

				writeError(w, http.StatusForbidden,
					"requests from another origin are not accepted")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// trimTicket removes a ticket from a URL for logging, so a credential in
// flight is never written to the log file.
func trimTicket(rawURL string) string {
	if idx := strings.Index(rawURL, "ticket="); idx >= 0 {
		return rawURL[:idx] + "ticket=REDACTED"
	}
	return rawURL
}
