package logging

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// The process-wide hook.
//
// A package-level default is kept for the same reason history.Default is: the
// readers are scattered -- the API serves the ring, the CLI prints it, the
// daemon closes it on shutdown -- and threading a hook through every
// constructor would widen a lot of signatures to carry one process-global
// thing. Until Install runs, Default is a hook with an unattached file, so
// reading logs in a unit test yields an empty list rather than a nil panic.
var (
	defaultMu sync.RWMutex
	def       = NewHook(NewRing(DefaultRingSize))
	installed bool
)

// Default returns the process-wide hook.
func Default() *Hook {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return def
}

// Install registers the default hook with logrus, once, and applies the given
// ring size and file options to it.
//
// It is safe to call repeatedly: config.InitLogger re-runs whenever the
// logging settings change, and only the first call adds the hook to logrus.
// Adding it twice would duplicate every entry.
func Install(ringSize int, rotate RotateOptions) error {
	defaultMu.Lock()
	hook := def
	first := !installed
	installed = true
	defaultMu.Unlock()

	if first {
		logrus.AddHook(hook)
	}

	if ringSize > 0 && hook.Ring().Cap() != ringSize {
		hook.SetRing(NewRing(ringSize))
	}

	writer, err := NewRotatingWriter(rotate)
	if err != nil {
		return err
	}
	hook.SetFile(writer)
	return nil
}

// Close releases the default hook's file. Called from the daemon's shutdown
// path so the last entries are flushed.
func Close() error {
	return Default().Close()
}
