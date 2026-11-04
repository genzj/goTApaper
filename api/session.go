package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/api/wire"
)

// Where the interface lives. Outside wire.BasePath, because it serves a user
// interface rather than the JSON API.
const uiPath = "/ui/"

// ticketExchangePath is where a ticket is traded for the API token.
const ticketExchangePath = wire.BasePath + "/auth/ticket"

const (
	// ticketTTL is how long a tray-minted ticket stays valid. Long enough for
	// a browser to cold-start, short enough that one left in a shell history
	// or a crash log is useless.
	ticketTTL = 60 * time.Second

	// ticketBytes is the entropy in a ticket before encoding.
	ticketBytes = 32
)

// ticketStore issues the single-use tickets that hand the interface its token.
//
// The interface authenticates like any other client, with the bearer token,
// which it keeps in the page's local storage. Opening it from the tray should
// not mean asking the user to paste that token, so the tray opens the page
// with a ticket instead, and the page trades it for the token.
//
// The ticket exists because the token itself must not ride in the URL. The
// browser is launched with that URL on its command line, which any local
// account can read with ps, and it is recorded in browser history. The token
// is long-lived and the token file is 0600 precisely so other accounts cannot
// read it; a spent, 60-second ticket in the same places is worthless.
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
	// URL-safe, since this travels in a URL fragment.
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

// ticketExchange trades a ticket for the API token.
//
// It is unauthenticated, because it is how the interface *acquires* its
// credential. That is not a hole: the ticket it demands is a single-use
// 60-second secret with 256 bits of entropy, minted by this process and
// handed only to a window or browser the tray opened.
type ticketExchange struct {
	tickets *ticketStore
	token   string
}

// ticketExchangeBody is the request and the response.
type ticketExchangeBody struct {
	Ticket string `json:"ticket,omitempty"`
	Token  string `json:"token,omitempty"`
}

func (e *ticketExchange) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body ticketExchangeBody
	if !decodeBody(w, r, &body) {
		return
	}

	if !e.tickets.Redeem(body.Ticket) {
		// Logged at warning for the same reason a rejected token is: this is
		// the user's only signal that something local is probing an interface
		// that can read their API keys.
		logrus.WithField("remote", r.RemoteAddr).Warn(
			"rejected a ticket exchange with no valid ticket",
		)
		writeError(w, http.StatusForbidden,
			"this link is invalid or has expired; open the interface from the tray menu again")
		return
	}

	// Never cached: the response carries the credential.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, ticketExchangeBody{Token: e.token})
}

// SessionURL mints a ticket and returns the URL that opens the interface
// already signed in.
//
// The ticket travels in the fragment rather than the query string. A fragment
// is never sent to the server, so it stays out of request logs, and it is
// stripped from the Referer header; the page reads it, trades it, and removes
// it from the address bar.
func (s *Server) SessionURL() (string, error) {
	ticket, err := s.tickets.Mint()
	if err != nil {
		return "", err
	}
	return s.UIURL() + "#ticket=" + url.QueryEscape(ticket), nil
}

// Token returns the API token, for handing to this process's own window.
//
// It exists for the native window only, which is in-process: the token is
// never served by any route except the ticket exchange.
func (s *Server) Token() string { return s.token }

// UIURL is the interface's plain address.
func (s *Server) UIURL() string {
	return "http://" + s.Addr() + uiPath
}

// isLoopbackHost reports whether a host, with or without a port, names this
// machine's loopback interface.
func isLoopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host == LoopbackHost || host == "localhost" || host == "::1"
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
	return isLoopbackHost(parsed.Host)
}

// requireLoopbackHost rejects a request addressed to any name but loopback.
//
// The bind already keeps other machines out. This keeps out a DNS-rebinding
// page: a site whose name the attacker re-points at 127.0.0.1 after the page
// loads, so the browser treats our server as that site's own origin. Such a
// request still names the attacker's host in its Host header. It matters now
// that the interface's assets and the ticket exchange answer without a token.
func requireLoopbackHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			logrus.WithFields(logrus.Fields{
				"host": r.Host,
				"path": r.URL.Path,
			}).Warn("rejected a request for a host other than loopback")

			writeError(w, http.StatusForbidden, "requests must be addressed to "+LoopbackHost)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireLoopbackOrigin rejects a cross-origin state-changing request.
//
// With the token sent in a header rather than a cookie, a hostile page cannot
// make an authenticated request at all. This stays as a second line, because
// the ticket exchange accepts a POST without a token.
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
