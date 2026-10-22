package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config/store"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// backupsKept is how many pre-migration database copies to retain.
const backupsKept = 2

type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads the embedded migrations, ordered by the numeric prefix
// of their filenames.
func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("migration %s is not named <version>_<name>.sql", e.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migration %s has a non-numeric version prefix: %w", e.Name(), err)
		}
		body, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: version, name: e.Name(), sql: string(body)})
	}

	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })

	if err := checkMigrations(ms); err != nil {
		return nil, err
	}
	return ms, nil
}

// checkMigrations validates a sorted migration set.
//
// This is a backstop; the real guard is TestMigrationsAreWellFormed, which
// runs the same check in CI so a bad set never reaches a user. Two migrations
// at one version would leave which of them applies up to filename order, and
// if they shipped in different releases the second would be skipped outright
// on a database already past that version.
func checkMigrations(ms []migration) error {
	for i, m := range ms {
		if i > 0 && m.version == ms[i-1].version {
			return fmt.Errorf(
				"migrations %s and %s share version %d",
				ms[i-1].name, m.name, m.version,
			)
		}
		if want := i + 1; m.version != want {
			return fmt.Errorf(
				"migration %s has version %d, want %d: versions must run consecutively from 1",
				m.name, m.version, want,
			)
		}
	}
	return nil
}

// migrate brings db up to the latest schema version.
//
// Migrations are up-only. There are no down-migrations: this is a single-user
// desktop application, so "downgrade" means someone installed an older build,
// not an orchestrated rollback, and a hand-written down-migration that drops a
// column is a data-loss bug waiting to happen. Two cheaper mechanisms cover the
// same ground -- a pre-migration backup of the database file, and refusing to
// open a database stamped newer than this binary understands.
//
// dbPath is the file being migrated, or "" for an in-memory database (which
// needs no backup, having nothing to lose). preexisting reports whether that
// file was already on disk before this process opened it; a database at
// version 0 is otherwise indistinguishable from one we just created, and
// backing up an empty file we made ourselves is noise.
func migrate(db *sql.DB, dbPath string, preexisting bool) error {
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		return fmt.Errorf("no embedded migrations found")
	}
	latest := ms[len(ms)-1].version

	current, err := userVersion(db)
	if err != nil {
		return err
	}

	if current > latest {
		// Operating on a schema we do not understand is how an older binary
		// corrupts a newer user's configuration. Refuse instead.
		return fmt.Errorf(
			"%w: database is at version %d but this build only knows up to %d; "+
				"upgrade goTApaper, or restore one of the %s.pre-v*.bak backups",
			store.ErrSchemaTooNew, current, latest, filepath.Base(dbPath),
		)
	}
	if current == latest {
		logrus.WithField("version", current).Debug("database schema is up to date")
		return nil
	}

	// Back up before touching an existing database, so a downgrade is just
	// "restore the backup" and needs no code.
	if dbPath != "" && preexisting {
		if err := backupDB(dbPath, current); err != nil {
			return fmt.Errorf("cannot back up database before migrating: %w", err)
		}
	}

	for _, m := range ms {
		if m.version <= current {
			continue
		}
		l := logrus.WithField("migration", m.name).WithField("version", m.version)
		l.Info("applying database migration")

		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.name, err)
		}
		l.Debug("migration applied")
	}
	return nil
}

// applyMigration runs one migration and stamps user_version atomically.
func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.sql); err != nil {
		return err
	}
	// PRAGMA does not accept a bound parameter, and the value is an int from a
	// filename we parsed ourselves, so interpolation is safe here.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return err
	}
	return tx.Commit()
}

func userVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("cannot read schema version: %w", err)
	}
	return v, nil
}

// backupDB copies the database file aside, naming the copy after the schema
// version it still holds.
func backupDB(dbPath string, version int) error {
	target := fmt.Sprintf("%s.pre-v%d.bak", dbPath, version)

	src, err := os.Open(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to back up; a fresh database has no history to lose.
			return nil
		}
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Sync(); err != nil {
		return err
	}

	logrus.WithField("backup", target).Info("database backed up before migration")
	pruneBackups(dbPath)
	return nil
}

// pruneBackups keeps only the most recent backupsKept copies. Failures are
// logged and ignored: a stale backup is clutter, not a reason to abort a
// migration that is otherwise fine.
func pruneBackups(dbPath string) {
	matches, err := filepath.Glob(dbPath + ".pre-v*.bak")
	if err != nil || len(matches) <= backupsKept {
		return
	}
	sort.Slice(matches, func(i, j int) bool {
		return backupVersion(matches[i]) > backupVersion(matches[j])
	})
	for _, stale := range matches[backupsKept:] {
		if err := os.Remove(stale); err != nil {
			logrus.WithField("backup", stale).WithError(err).Debug("cannot prune old backup")
		}
	}
}

// backupVersion extracts N from a "<db>.pre-vN.bak" filename, returning -1 if
// the name does not parse so unrecognised files sort last and get pruned first.
func backupVersion(name string) int {
	_, rest, ok := strings.Cut(filepath.Base(name), ".pre-v")
	if !ok {
		return -1
	}
	digits, _, ok := strings.Cut(rest, ".")
	if !ok {
		return -1
	}
	v, err := strconv.Atoi(digits)
	if err != nil {
		return -1
	}
	return v
}
