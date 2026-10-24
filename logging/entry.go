// Package logging keeps the most recent log output available for reading.
//
// logrus writes to stderr, which is unreachable once goTApaper runs as a tray
// application: there is no terminal attached to look at. This package adds a
// logrus hook that keeps the recent entries in memory for the UI to read and
// tail, and writes the same entries to a rotating file so a problem can still
// be investigated after a restart.
//
// Both stores are bounded by size rather than by age. A log line goes away
// because something newer needed the room, which is one rule rather than two
// that could disagree.
package logging

import (
	"encoding/json"
	"time"
)

// Entry is one log record, flattened into the shape the API serves and the
// rotating file stores.
//
// It is a copy rather than a reference to the logrus entry it came from:
// logrus reuses entry objects, so retaining one in the ring would let a later
// write mutate a record the UI is already displaying.
type Entry struct {
	// Seq numbers entries within one process run, so a client that has already
	// seen up to N can ask for what came after without relying on timestamps,
	// which are not unique at debug volume.
	Seq uint64 `json:"seq"`
	// Time is when the entry was logged, in UTC.
	Time time.Time `json:"time"`
	// Level is the lowercase logrus level name ("info", "debug", ...).
	Level string `json:"level"`
	// Message is the log message.
	Message string `json:"message"`
	// Fields holds the structured fields, already rendered to strings. The
	// values logrus carries are arbitrary types, and a UI only ever displays
	// them, so they are flattened once here instead of at every read.
	Fields map[string]string `json:"fields,omitempty"`
}

// MarshalJSONLine renders the entry as one newline-terminated JSON object, the
// format written to the rotating log file.
func (e Entry) MarshalJSONLine() ([]byte, error) {
	encoded, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// Filter narrows a ring snapshot. The zero value matches everything.
type Filter struct {
	// MinLevel drops entries less severe than this logrus level name. Empty
	// means no level filtering.
	MinLevel string
	// Since returns only entries with a sequence number greater than this, so
	// a client can poll for what it has not seen. Zero means from the start of
	// the buffer.
	Since uint64
	// Limit caps the number of entries returned, keeping the *most recent*
	// ones. Zero or negative means no limit.
	Limit int
}

// levelSeverity orders the logrus level names from most to least severe. A
// table rather than logrus.ParseLevel because an unknown name here should
// simply not filter anything, not raise an error on every read.
var levelSeverity = map[string]int{
	"panic": 0,
	"fatal": 1,
	"error": 2,
	"warn":  3,
	// logrus accepts both spellings and emits "warning".
	"warning": 3,
	"info":    4,
	"debug":   5,
	"trace":   6,
}

// Matches reports whether an entry passes the filter's level and sequence
// tests. Limit is applied by the caller, since it depends on the whole set.
func (f Filter) Matches(e Entry) bool {
	if e.Seq <= f.Since {
		return false
	}
	if f.MinLevel == "" {
		return true
	}

	want, ok := levelSeverity[f.MinLevel]
	if !ok {
		// An unrecognised level is treated as no filter rather than as a
		// filter matching nothing, so a typo in a query string does not look
		// like "there are no logs".
		return true
	}
	have, ok := levelSeverity[e.Level]
	if !ok {
		return true
	}
	return have <= want
}
