package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/api/wire"
)

// tokenBytes is the length of a generated token before hex encoding.
const tokenBytes = 32

// TokenPath returns the token file's location.
func TokenPath(appDir string) string { return wire.TokenPath(appDir) }

// EndpointPath returns the endpoint file's location.
func EndpointPath(appDir string) string { return wire.EndpointPath(appDir) }

// loadOrCreateToken reads the API token, generating one on first use.
//
// The API mutates configuration, triggers outbound downloads, and holds the
// user's provider API keys, so loopback alone is not an authorisation
// boundary: on a shared machine any local account could otherwise drive it.
// The file is 0600 so only its owner can read it.
//
// If two daemons start at once they both generate a token and the last write
// wins, leaving the other serving a credential no longer on disk. That is
// accepted rather than defended against: a single user does not normally run
// two daemons, and the consequence is mild. A client holding the superseded
// token gets a 401, and the CLI treats an unusable token as "no daemon
// running" and writes to the database directly, which still works.
func loadOrCreateToken(appDir string) (string, error) {
	path := wire.TokenPath(appDir)

	token, err := readToken(path)
	if err != nil {
		return "", err
	}
	if token != "" {
		return token, nil
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("cannot generate an API token: %w", err)
	}
	token = hex.EncodeToString(raw)

	if err := os.WriteFile(path, []byte(token+"\n"), 0600); err != nil {
		return "", fmt.Errorf("cannot write the API token: %w", err)
	}
	// WriteFile only applies its mode when creating, so an existing file keeps
	// whatever permissions it had. Tighten it explicitly: the restriction is
	// the point of this file.
	if err := os.Chmod(path, 0600); err != nil {
		return "", fmt.Errorf("cannot restrict the API token file: %w", err)
	}

	logrus.WithField("file", path).Info("generated a new API token")
	return token, nil
}

// readToken returns the stored token, or "" if the file is absent or holds no
// usable token. An empty file can be left behind by an interrupted write, and
// is treated as absent so the caller simply generates a new token.
func readToken(path string) (string, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the API token: %w", err)
	}

	token := strings.TrimSpace(string(content))
	if token == "" {
		logrus.WithField("file", path).Warn(
			"the API token file is empty; a new token will be generated",
		)
	}
	return token, nil
}

// requireToken wraps a handler with bearer token authentication.
//
// A request made directly from this machine may go without a token unless
// strict says otherwise (api.require-token). Everything else needs it:
//
//   - a request carrying a token is always checked, so a wrong one is reported
//     rather than silently ignored;
//   - a request a proxy forwarded counts as remote, whatever address it
//     arrives from. A reverse proxy on this machine connects from 127.0.0.1,
//     so the source address alone would let through everyone the proxy does.
//     This is best-effort: it relies on the proxy setting a forwarding header,
//     which is why it is worth setting api.require-token behind one.
//
// Tokenless local access leans on the other guards: the bind admits nothing
// from other machines, requireLoopbackHost turns away DNS-rebinding pages, and
// requireLoopbackOrigin turns away cross-origin writes, which is what stops a
// web page in the user's browser from driving the API.
func requireToken(token string, strict func() bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, ok := bearerToken(r)
		if !ok && !strict() && isDirectLocalRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Constant time, so a wrong token cannot be narrowed down by timing
		// the response.
		if !ok || subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			// Logged at warning rather than debug: this is the only signal a
			// user gets that something on their machine is probing an
			// interface that can read their API keys. The presented token is
			// deliberately not logged.
			logrus.WithFields(logrus.Fields{
				"method":     r.Method,
				"path":       r.URL.Path,
				"remote":     r.RemoteAddr,
				"credential": describeCredential(ok),
			}).Warn("rejected an unauthorized API request")

			w.Header().Set("WWW-Authenticate", `Bearer realm="goTApaper"`)
			writeError(w, http.StatusUnauthorized, "a valid bearer token is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// forwardingHeaders mark a request a proxy passed on.
var forwardingHeaders = []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Real-Ip"}

// isDirectLocalRequest reports whether a request came straight from this
// machine rather than through a proxy.
func isDirectLocalRequest(r *http.Request) bool {
	for _, h := range forwardingHeaders {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// describeCredential says whether a token was offered at all, which is the
// difference between a misconfigured client and a probe.
func describeCredential(presented bool) string {
	if presented {
		return "incorrect"
	}
	return "missing"
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}
