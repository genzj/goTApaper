package cmd

import (
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/api"
	"github.com/genzj/goTApaper/util"
)

// lifecycle stops and restarts a running daemon, headless or windowed.
//
// Stopping is the same in both: ask whatever owns the process to return. What
// that means differs -- leaving a wait for the headless daemon, quitting the
// UI event loop for the interface -- so it is supplied as quit. Restarting is
// stopping with a note to start again once everything is released.
type lifecycle struct {
	quit    func()
	once    sync.Once
	restart atomic.Bool
}

var _ api.Lifecycle = (*lifecycle)(nil)

func newLifecycle(quit func()) *lifecycle {
	return &lifecycle{quit: quit}
}

// Stop shuts the process down. Repeated calls are ignored, so a signal
// arriving during an API-requested stop cannot quit twice.
func (l *lifecycle) Stop() {
	l.once.Do(func() {
		// In its own goroutine: an API handler calls this, and quitting the
		// UI waits for the server -- and so that handler -- to finish.
		go l.quit()
	})
}

// Restart shuts the process down and starts it again.
func (l *lifecycle) Restart() {
	logrus.Info("restarting")
	l.restart.Store(true)
	l.Stop()
}

// restartIfRequested starts the process again if Restart asked for it. It is
// called last, once the API server, the log and -- for the interface -- the
// single-instance lock have all been released, so the new process can take
// them. On Unix it does not return: the process is replaced in place.
func (l *lifecycle) restartIfRequested() {
	if !l.restart.Load() {
		return
	}
	// The store is closed so the replacement opens a database whose
	// write-ahead log has been checkpointed, rather than recovering one.
	if provider != nil {
		if err := provider.Store().Close(); err != nil {
			logrus.WithError(err).Debug("cannot close the configuration database")
		}
	}
	if err := util.Reexec(); err != nil {
		logrus.WithError(err).Error("cannot restart; start goTApaper again by hand")
	}
}
