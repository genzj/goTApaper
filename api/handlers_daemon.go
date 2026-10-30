package api

import "net/http"

// Lifecycle stops or restarts the process serving the API.
//
// It is an interface rather than a function the server calls directly,
// because how to stop depends on how the process runs: a windowed one quits
// its UI event loop, a headless one leaves its wait.
type Lifecycle interface {
	// Stop shuts the process down. It returns at once; the shutdown happens
	// after the response is written.
	Stop()
	// Restart shuts the process down and starts it again.
	Restart()
}

// registerDaemonRoutes serves stopping and restarting the daemon.
func (h *handlers) registerDaemonRoutes(r *router) {
	r.handle("POST", "/daemon/restart", h.restartDaemon)
	r.handle("POST", "/daemon/stop", h.stopDaemon)
}

// restartDaemon answers 202 and then restarts. The process comes back on the
// same port (see listen), so a client that keeps its token can reconnect.
func (h *handlers) restartDaemon(w http.ResponseWriter, r *http.Request) {
	if h.lifecycle == nil {
		writeError(w, http.StatusNotImplemented, "this process cannot be restarted")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
	// Deferred until the response has been sent, or the client would see a
	// dropped connection instead of an answer.
	flush(w)
	h.lifecycle.Restart()
}

// stopDaemon answers 202 and then stops. The interface uses restart; this is
// how a starting GUI takes over from a headless daemon serving the same
// application directory, so the database never has two writers.
func (h *handlers) stopDaemon(w http.ResponseWriter, r *http.Request) {
	if h.lifecycle == nil {
		writeError(w, http.StatusNotImplemented, "this process cannot be stopped")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	flush(w)
	h.lifecycle.Stop()
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
