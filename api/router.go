package api

import (
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/genzj/goTApaper/api/wire"
)

// router registers routes on a ServeMux while remembering which methods each
// path pattern accepts.
//
// The bookkeeping exists because the catch-all handlers needed to produce JSON
// errors also suppress the mux's built-in 405: once "/api/v1/" is registered,
// a request whose path matches a route but whose method does not simply falls
// through to the prefix instead of being reported as a method mismatch. The
// table lets unmatched tell the two cases apart again, so a client can
// distinguish "wrong verb" from "wrong URL".
type router struct {
	mux     *http.ServeMux
	allowed map[string][]string
}

// handle registers one route. path is relative to wire.BasePath.
func (r *router) handle(method, path string, h http.HandlerFunc) {
	full := wire.BasePath + path
	r.mux.HandleFunc(method+" "+full, h)

	if !slices.Contains(r.allowed[full], method) {
		r.allowed[full] = append(r.allowed[full], method)
		sort.Strings(r.allowed[full])
	}
}

// unmatched answers a request that reached no route: 405 when the path exists
// under other methods, 404 otherwise.
func (r *router) unmatched(w http.ResponseWriter, req *http.Request) {
	if methods := r.methodsFor(req.URL.Path); len(methods) > 0 {
		// Allow is required on a 405 by RFC 9110, and it is what makes the
		// response actionable rather than just a refusal.
		w.Header().Set("Allow", strings.Join(methods, ", "))
		writeError(w, http.StatusMethodNotAllowed, "method "+req.Method+
			" is not allowed on "+req.URL.Path+"; allowed: "+strings.Join(methods, ", "))
		return
	}

	writeError(w, http.StatusNotFound, "no such endpoint: "+req.Method+" "+req.URL.Path)
}

// methodsFor returns the methods registered for a concrete request path,
// matching wildcard segments in the registered patterns.
func (r *router) methodsFor(path string) []string {
	for pattern, methods := range r.allowed {
		if patternMatches(pattern, path) {
			return methods
		}
	}
	return nil
}

// patternMatches compares a registered pattern against a concrete path,
// treating a "{name}" segment as a single-segment wildcard. This mirrors the
// subset of ServeMux's syntax actually used above; it deliberately does not
// implement "{name...}", since no route needs it.
func patternMatches(pattern, path string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		return false
	}
	for i, want := range patternParts {
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			// A wildcard must still consume something, or "/channels/" would
			// look like a match for "/channels/{name}".
			if pathParts[i] == "" {
				return false
			}
			continue
		}
		if want != pathParts[i] {
			return false
		}
	}
	return true
}
