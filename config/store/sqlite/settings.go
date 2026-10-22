package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// Setting returns one setting, falling back to the in-code default when the
// user has never set it. Only a key missing from the schema is an error.
func (s *Store) Setting(key string) (store.Setting, error) {
	def, ok := config.Lookup(key)
	if !ok {
		return store.Setting{}, fmt.Errorf("%w: %s", config.ErrUnknownKey, key)
	}

	var value sql.NullString
	err := s.q().QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows) || !value.Valid:
		return store.Setting{Key: key, Value: def.Default, IsDefault: true}, nil
	case err != nil:
		return store.Setting{}, err
	}
	return store.Setting{Key: key, Value: value.String}, nil
}

// Settings returns every setting in the schema, stored or defaulted.
//
// The schema drives the iteration, not the table: a row whose key is no longer
// in the schema is skipped with a warning rather than surfaced. That is what
// lets a database written by a newer build still open in an older one.
func (s *Store) Settings() ([]store.Setting, error) {
	stored, err := s.storedSettings()
	if err != nil {
		return nil, err
	}

	defs := config.Definitions()
	out := make([]store.Setting, 0, len(defs))
	for _, def := range defs {
		if value, ok := stored[def.Key]; ok {
			out = append(out, store.Setting{Key: def.Key, Value: value})
			delete(stored, def.Key)
			continue
		}
		out = append(out, store.Setting{Key: def.Key, Value: def.Default, IsDefault: true})
	}

	for key := range stored {
		logrus.WithField("key", key).Warn(
			"ignoring unrecognised setting in database; " +
				"it may belong to a newer version of goTApaper",
		)
	}
	return out, nil
}

func (s *Store) storedSettings() (map[string]string, error) {
	rows, err := s.q().Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stored := map[string]string{}
	for rows.Next() {
		var key string
		var value sql.NullString
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if value.Valid {
			stored[key] = value.String
		}
	}
	return stored, rows.Err()
}

// SetSetting validates value against the key's definition and stores it.
// Validation happens here, in Go, against config.Definitions -- the database
// holds no copy of the valid value sets.
func (s *Store) SetSetting(key, value string) error {
	if err := config.Validate(key, value); err != nil {
		return err
	}

	old, err := s.Setting(key)
	if err != nil {
		return err
	}

	_, err = s.q().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, unixepoch())
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value,
	)
	if err != nil {
		return err
	}

	s.emit(key, old.Value, value)
	return nil
}

// UnsetSetting removes a stored value, reverting the key to its default.
func (s *Store) UnsetSetting(key string) error {
	def, ok := config.Lookup(key)
	if !ok {
		return fmt.Errorf("%w: %s", config.ErrUnknownKey, key)
	}

	old, err := s.Setting(key)
	if err != nil {
		return err
	}

	if _, err := s.q().Exec("DELETE FROM settings WHERE key = ?", key); err != nil {
		return err
	}

	s.emit(key, old.Value, def.Default)
	return nil
}
