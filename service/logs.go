package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/logging"
)

// DefaultLogPageSize is how many entries a log read returns when the caller
// does not say.
const DefaultLogPageSize = 200

// Logs exposes the recent log output.
//
// It reads the process-wide hook rather than holding one, because the hook is
// installed by config.InitLogger long before any service exists and is
// replaced whenever the logging settings change.
type Logs struct{}

// NewLogs builds a log service.
func NewLogs() *Logs { return &Logs{} }

// LogPage is one read of the in-memory log buffer.
type LogPage struct {
	Entries []logging.Entry `json:"entries"`
	// LastSeq is the sequence number of the newest entry returned, so a
	// client can poll for what came after it.
	LastSeq uint64 `json:"lastSeq"`
	// Capacity is how many entries the buffer holds in total, which tells a
	// client whether a gap means "nothing happened" or "we fell behind".
	Capacity int `json:"capacity"`
}

// Read returns buffered log entries, oldest first.
func (l *Logs) Read(minLevel string, since uint64, limit int) LogPage {
	if limit <= 0 {
		limit = DefaultLogPageSize
	}

	ring := logging.Default().Ring()
	entries := ring.Snapshot(logging.Filter{
		MinLevel: minLevel,
		Since:    since,
		Limit:    limit,
	})

	page := LogPage{Entries: entries, Capacity: ring.Cap()}
	if len(entries) > 0 {
		page.LastSeq = entries[len(entries)-1].Seq
	} else {
		page.LastSeq = since
	}
	return page
}

// Follow subscribes to entries logged from now on. The returned function ends
// the subscription.
func (l *Logs) Follow(buffer int) (<-chan logging.Entry, func()) {
	return logging.Default().Ring().Subscribe(buffer)
}

// Files lists the log files on disk, newest first, with the active one first.
func (l *Logs) Files() ([]logging.FileInfo, error) {
	return logging.Files(config.GetLogFileName())
}

// OpenFile opens one log file by name, for download. The caller closes it.
//
// Only files the listing reports are openable. Resolving an arbitrary name
// against the log directory would otherwise let a caller walk out of it with
// "../../.ssh/id_rsa" -- the API is authenticated, but a path traversal is
// still a path traversal, and the listing is the authoritative set.
func (l *Logs) OpenFile(name string) (*os.File, error) {
	if name != filepath.Base(name) {
		return nil, fmt.Errorf("%w: log file %q", store.ErrNotFound, name)
	}

	files, err := l.Files()
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Name == name {
			return os.Open(f.Path)
		}
	}
	return nil, fmt.Errorf("%w: log file %q", store.ErrNotFound, name)
}
