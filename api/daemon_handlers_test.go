package api

import (
	"net/http"
	"sync/atomic"
	"testing"
)

type recordingLifecycle struct {
	stops, restarts atomic.Int32
}

func (r *recordingLifecycle) Stop()    { r.stops.Add(1) }
func (r *recordingLifecycle) Restart() { r.restarts.Add(1) }

func TestDaemonRestartAndStopAnswerThenAct(t *testing.T) {
	handlers, _ := newTestHandlers(t, nil)
	lc := &recordingLifecycle{}
	handlers.lifecycle = lc
	h := testRoutes(handlers, newTicketStore())

	// 202, and the action taken after answering, so the client hears back
	// rather than seeing its connection drop.
	if rec := post(t, h, "/api/v1/daemon/restart", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("restart: status = %d", rec.Code)
	}
	if rec := post(t, h, "/api/v1/daemon/stop", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("stop: status = %d", rec.Code)
	}
	if lc.restarts.Load() != 1 || lc.stops.Load() != 1 {
		t.Fatalf("restarts/stops = %d/%d, want 1/1", lc.restarts.Load(), lc.stops.Load())
	}
}

func TestDaemonLifecycleRoutesRequireTheToken(t *testing.T) {
	handlers, _ := newTestHandlers(t, nil)
	lc := &recordingLifecycle{}
	handlers.lifecycle = lc
	h := testRoutes(handlers, newTicketStore())

	// Stopping the daemon is as sensitive as anything else it does.
	for _, path := range []string{"/api/v1/daemon/restart", "/api/v1/daemon/stop"} {
		if rec := do(t, h, "POST", path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: status = %d, want 401", path, rec.Code)
		}
	}
	if lc.restarts.Load()+lc.stops.Load() != 0 {
		t.Fatal("an unauthenticated request stopped or restarted the daemon")
	}
}

func TestDaemonLifecycleWithoutOneIsNotImplemented(t *testing.T) {
	h, _ := newTestServer(t)
	if rec := post(t, h, "/api/v1/daemon/restart", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}
