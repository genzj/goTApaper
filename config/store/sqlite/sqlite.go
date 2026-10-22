// Package sqlite is the primary store implementation, backed by a local
// SQLite database.
//
// The driver is modernc.org/sqlite, a pure-Go translation rather than a cgo
// binding. That matters here: goTApaper cross-compiles to Windows, macOS and
// Linux, and the build already carries cgo for the system tray and the
// wallpaper setters. Adding a second cgo dependency would make that matrix
// meaningfully worse.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"

	// Registers the "sqlite" driver.
	_ "modernc.org/sqlite"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// MemoryPath opens a private in-memory database, used by tests.
const MemoryPath = ":memory:"

// memorySeq names successive in-memory databases so concurrent Opens do not
// land on the same one.
var memorySeq atomic.Uint64

func nextMemoryID() uint64 { return memorySeq.Add(1) }

// Store is a SQLite-backed store.Store.
type Store struct {
	db *sql.DB
	// tx is non-nil when this Store is a transactional view created by Tx.
	tx *sql.Tx
	// path is the database file, empty for in-memory databases.
	path string
	// pending holds change events buffered inside a transaction, flushed
	// after it commits.
	pending []changeEvent
}

// changeEvent is a configuration change awaiting publication.
type changeEvent struct {
	key      string
	old, new any
}

var _ store.Store = (*Store)(nil)

// Open opens (creating if necessary) the database at path and migrates it to
// the latest schema. Pass MemoryPath for a throwaway in-memory database.
func Open(path string) (*Store, error) {
	dsn := path
	filePath := path
	if path == MemoryPath {
		// A shared cache keeps the database alive across pooled connections;
		// without it each connection would get its own empty database. The
		// name must be unique per Open, though: "file::memory:?cache=shared"
		// is process-global, so two callers would silently share one database.
		dsn = fmt.Sprintf("file:gotapaper-%d?mode=memory&cache=shared", nextMemoryID())
		filePath = ""
	}

	// Checked before opening, because sql.Open followed by any query creates
	// the file and we would no longer be able to tell whether the user had a
	// database here already.
	preexisting := false
	if filePath != "" {
		if info, err := os.Stat(filePath); err == nil && info.Size() > 0 {
			preexisting = true
		}
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot open configuration database: %w", err)
	}

	if err := applyPragmas(db, filePath == ""); err != nil {
		db.Close()
		return nil, err
	}

	if err := migrate(db, filePath, preexisting); err != nil {
		db.Close()
		return nil, err
	}

	s := &Store{db: db, path: filePath}

	// Table structure is now current; check the *content* schema too, which
	// covers the meaning of what is stored rather than its shape.
	if err := s.checkContentSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func applyPragmas(db *sql.DB, inMemory bool) error {
	pragmas := []string{
		// Required, not cosmetic: without it SQLite ignores the ON DELETE
		// RESTRICT that protects in-use channel templates.
		"PRAGMA foreign_keys = ON",
		// The daemon and a CLI invocation can touch the database at once.
		"PRAGMA busy_timeout = 5000",
	}
	if !inMemory {
		// WAL needs a real file; it is both unnecessary and unsupported for
		// an in-memory database.
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("cannot apply %q: %w", p, err)
		}
	}

	// An in-memory database lives only as long as its connections, so the pool
	// must never drop to zero or the schema vanishes mid-test.
	if inMemory {
		db.SetMaxOpenConns(1)
	}
	return nil
}

// Close releases the database. It is a no-op on a transactional view, whose
// lifetime belongs to the enclosing Tx call.
func (s *Store) Close() error {
	if s.tx != nil {
		return nil
	}
	return s.db.Close()
}

// Tx runs fn against a transactional view of the store, committing on success
// and rolling back on error or panic. Nested calls reuse the open transaction
// so a caller does not have to know whether it is already inside one.
func (s *Store) Tx(fn func(store.Store) error) error {
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	txStore := &Store{db: s.db, tx: tx, path: s.path}

	committed := false
	defer func() {
		if !committed {
			// Rollback after a successful commit returns ErrTxDone, which is
			// why this is guarded rather than unconditional.
			tx.Rollback()
		}
	}()

	if err := fn(txStore); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	// Only now is the change real, so only now may observers see it. A
	// listener that reacted mid-transaction could read state that a later
	// rollback erased -- and the daemon's listener triggers a refresh.
	txStore.flush()
	return nil
}

// emit publishes a configuration change, or defers it until commit when inside
// a transaction.
func (s *Store) emit(key string, old, new any) {
	if s.tx != nil {
		s.pending = append(s.pending, changeEvent{key: key, old: old, new: new})
		return
	}
	config.Emit(key, old, new)
}

// flush publishes the events buffered during a transaction.
func (s *Store) flush() {
	for _, e := range s.pending {
		config.Emit(e.key, e.old, e.new)
	}
	s.pending = nil
}

// querier is the subset of *sql.DB and *sql.Tx this package uses, so every
// query runs against the transaction when there is one.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func (s *Store) q() querier {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}
