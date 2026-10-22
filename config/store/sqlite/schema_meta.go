package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// Content schema names tracked in schema_meta.
const (
	// MetaSettingsSchema versions the set of setting keys and their meaning.
	MetaSettingsSchema = "settings"
)

// SchemaVersion returns the recorded version of a content schema, or 0 if it
// has never been recorded.
func (s *Store) SchemaVersion(name string) (int, error) {
	var version int
	err := s.q().QueryRow(
		"SELECT version FROM schema_meta WHERE name = ?", name,
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the %s schema version: %w", name, err)
	}
	return version, nil
}

// SetSchemaVersion records a content schema's version.
func (s *Store) SetSchemaVersion(name string, version int) error {
	_, err := s.q().Exec(
		`INSERT INTO schema_meta (name, version, updated_at)
		 VALUES (?, ?, unixepoch())
		 ON CONFLICT(name) DO UPDATE SET
		     version = excluded.version, updated_at = excluded.updated_at`,
		name, version,
	)
	if err != nil {
		return fmt.Errorf("cannot record the %s schema version: %w", name, err)
	}
	return nil
}

// checkContentSchema compares the stored content schema versions against what
// this build understands.
//
// A database written by a newer build is refused rather than opened. The
// alternative is worse than failing: unknown setting keys would be ignored and
// renamed ones would read as absent, so the application would start up
// applying a configuration the user never asked for.
//
// An older version is adopted silently. There is nothing to migrate yet, and
// when there is, this is where the Go-side migration belongs -- content
// changes such as renaming an option or splitting one value into two are not
// expressible as SQL, which is why the version lives in a row rather than in
// PRAGMA user_version.
func (s *Store) checkContentSchema() error {
	stored, err := s.SchemaVersion(MetaSettingsSchema)
	if err != nil {
		return err
	}

	switch {
	case stored > config.SettingsSchemaVersion:
		return fmt.Errorf(
			"%w: the settings schema is at version %d but this build only understands %d",
			store.ErrSchemaTooNew, stored, config.SettingsSchemaVersion,
		)

	case stored == 0:
		// A database this build just created, or one from before content
		// versions were recorded. Stamp it.
		return s.SetSchemaVersion(MetaSettingsSchema, config.SettingsSchemaVersion)

	case stored < config.SettingsSchemaVersion:
		logrus.WithFields(logrus.Fields{
			"stored":  stored,
			"current": config.SettingsSchemaVersion,
		}).Info("upgrading the settings schema version")
		return s.SetSchemaVersion(MetaSettingsSchema, config.SettingsSchemaVersion)
	}
	return nil
}
