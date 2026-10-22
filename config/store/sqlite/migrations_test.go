package sqlite

import (
	"testing"
)

// TestMigrationsAreWellFormed validates the embedded migration set at test
// time, so a mistake fails CI rather than a user's first run.
//
// Two migrations sharing a version is the case worth catching early: only one
// of them would ever be applied, and which one is decided by filename order
// rather than by intent. If they were added in different releases, the second
// would simply be skipped on an already-migrated database, leaving a schema
// the code expects but the file does not have. The runtime check in migrate()
// stays as a backstop, but this is where the mistake should surface.
func TestMigrationsAreWellFormed(t *testing.T) {
	ms, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations = %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations are embedded")
	}

	seen := map[int]string{}
	for i, m := range ms {
		if previous, duplicate := seen[m.version]; duplicate {
			t.Errorf("migrations %s and %s share version %d; "+
				"only one would ever be applied", previous, m.name, m.version)
		}
		seen[m.version] = m.name

		// Contiguous from 1, so there is no gap that a later migration could
		// be dropped into and silently skipped on a database already past it.
		if want := i + 1; m.version != want {
			t.Errorf("migration %s has version %d, want %d: versions must run "+
				"consecutively from 1", m.name, m.version, want)
		}

		if m.sql == "" {
			t.Errorf("migration %s is empty", m.name)
		}
	}
}

// TestLoadMigrationsRejectsDuplicateVersions covers the backstop itself, since
// the embedded set is (correctly) never in that state.
func TestLoadMigrationsRejectsDuplicateVersions(t *testing.T) {
	ms := []migration{
		{version: 1, name: "0001_a.sql", sql: "SELECT 1;"},
		{version: 1, name: "0001_b.sql", sql: "SELECT 1;"},
	}
	if err := checkMigrations(ms); err == nil {
		t.Error("two migrations at version 1 should be rejected")
	}

	ms = []migration{
		{version: 1, name: "0001_a.sql", sql: "SELECT 1;"},
		{version: 3, name: "0003_c.sql", sql: "SELECT 1;"},
	}
	if err := checkMigrations(ms); err == nil {
		t.Error("a gap in the version sequence should be rejected")
	}

	ms = []migration{
		{version: 1, name: "0001_a.sql", sql: "SELECT 1;"},
		{version: 2, name: "0002_b.sql", sql: "SELECT 1;"},
	}
	if err := checkMigrations(ms); err != nil {
		t.Errorf("a well-formed set was rejected: %v", err)
	}
}
