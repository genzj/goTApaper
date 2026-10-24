package logging

import (
	"fmt"
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

// Hook feeds every logrus entry into a Ring and, when one is configured, into
// a rotating file.
//
// It is a hook rather than a logrus formatter or a second io.Writer because a
// hook sees the structured fields before they are rendered, which is what lets
// the API serve them as JSON instead of re-parsing a formatted line.
type Hook struct {
	mu   sync.RWMutex
	ring *Ring
	// file receives one JSON line per entry. Nil disables file logging, which
	// is the state during tests and before configuration has been loaded.
	file io.WriteCloser
}

var _ logrus.Hook = (*Hook)(nil)

// NewHook builds a hook over a ring. Attach a file with SetFile.
func NewHook(ring *Ring) *Hook {
	if ring == nil {
		ring = NewRing(DefaultRingSize)
	}
	return &Hook{ring: ring}
}

// Ring returns the buffer the hook writes to.
func (h *Hook) Ring() *Ring {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ring
}

// SetFile installs the rotating writer, closing any previous one. A nil writer
// turns file logging off.
//
// Replacing rather than reconfiguring keeps this callable from
// config.InitLogger, which re-runs whenever the logging settings change.
func (h *Hook) SetFile(w io.WriteCloser) {
	h.mu.Lock()
	previous := h.file
	h.file = w
	h.mu.Unlock()

	if previous != nil {
		// Closed outside the lock: a close flushes, and a flush must not hold
		// up the goroutines that are logging.
		_ = previous.Close()
	}
}

// SetRing replaces the in-memory buffer, used when log.ring-size changes.
// Buffered entries are not carried over: the new size is a statement about how
// much history to keep, and copying would briefly hold two buffers.
func (h *Hook) SetRing(ring *Ring) {
	if ring == nil {
		return
	}
	h.mu.Lock()
	h.ring = ring
	h.mu.Unlock()
}

// Close releases the rotating file.
func (h *Hook) Close() error {
	h.mu.Lock()
	file := h.file
	h.file = nil
	h.mu.Unlock()

	if file == nil {
		return nil
	}
	return file.Close()
}

// Levels reports the levels the hook fires on: all of them.
//
// Filtering by level is the logger's job, not the hook's -- logrus only calls
// a hook for entries that already passed the configured level -- so returning
// every level here simply means "whatever the logger admits".
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire records one entry. Errors are swallowed deliberately: a logging
// failure must not become an application failure, and returning an error here
// would make logrus write a complaint to stderr for every line.
func (h *Hook) Fire(entry *logrus.Entry) error {
	h.mu.RLock()
	ring, file := h.ring, h.file
	h.mu.RUnlock()

	stored := ring.Add(convert(entry))

	if file == nil {
		return nil
	}
	line, err := stored.MarshalJSONLine()
	if err != nil {
		return nil
	}
	_, _ = file.Write(line)
	return nil
}

// convert copies a logrus entry into an Entry, flattening its fields to
// strings.
func convert(entry *logrus.Entry) Entry {
	e := Entry{
		Time:    entry.Time.UTC(),
		Level:   entry.Level.String(),
		Message: entry.Message,
	}

	if len(entry.Data) > 0 {
		e.Fields = make(map[string]string, len(entry.Data))
		for k, v := range entry.Data {
			// %v rather than a type switch: these values are only ever
			// displayed, and an error's own Error() is what %v yields.
			e.Fields[k] = fmt.Sprintf("%v", v)
		}
	}
	return e
}
