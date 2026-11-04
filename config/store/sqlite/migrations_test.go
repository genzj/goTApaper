package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/genzj/goTApaper/config/store"
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

// openAtVersion builds a database file migrated only up to version, so a test
// can seed it with data the later migrations have to carry forward.
func openAtVersion(t *testing.T, version int) (*sql.DB, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gotapaper.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ms, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range ms {
		if m.version > version {
			break
		}
		if err := applyMigration(db, m); err != nil {
			t.Fatalf("applying %s: %v", m.name, err)
		}
	}
	return db, path
}

func TestMigration3CarriesDownloadHistoryIntoTheWallpaperHistory(t *testing.T) {
	db, path := openAtVersion(t, 2)

	// The seeded channels are ng (ng-photo-of-today) and bing
	// (bing-wallpaper). Add two instances of one type to make that type key
	// ambiguous.
	for _, stmt := range []string{
		`INSERT INTO channel_instances (name, type, sort_order) VALUES
		     ('nature', 'unsplash', 2), ('city', 'unsplash', 3)`,
		// Keyed by type, as the SQLite-era code wrote it: unambiguous.
		`INSERT INTO download_history (channel_name, url, title, download_time)
		 VALUES ('bing-wallpaper', 'https://example.invalid/bing.jpg', 'Bing', 1000)`,
		// Keyed by instance name, as an imported history.json could be.
		`INSERT INTO download_history (channel_name, url, download_time)
		 VALUES ('ng', 'https://example.invalid/ng.jpg', 1001)`,
		// Keyed by a type two instances share: no telling whose it was.
		`INSERT INTO download_history (channel_name, url, download_time)
		 VALUES ('unsplash', 'https://example.invalid/unsplash.jpg', 1002)`,
		// Keyed by nothing that still exists.
		`INSERT INTO download_history (channel_name, url, download_time)
		 VALUES ('removed-long-ago', 'https://example.invalid/gone.jpg', 1003)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seeding: %v\n%s", err, stmt)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	for channel, want := range map[string]string{
		"bing":   "https://example.invalid/bing.jpg",
		"ng":     "https://example.invalid/ng.jpg",
		"nature": "",
		"city":   "",
	} {
		got, err := s.LastAppliedURL(channel)
		if err != nil {
			t.Fatalf("LastAppliedURL(%s): %v", channel, err)
		}
		if got != want {
			t.Errorf("LastAppliedURL(%s) = %q, want %q", channel, got, want)
		}
	}

	// The carried row keeps its metadata and its original time.
	bing, err := s.WallpaperHistory(0, 0)
	if err != nil {
		t.Fatalf("WallpaperHistory: %v", err)
	}
	if len(bing) != 2 {
		t.Fatalf("carried %d rows, want 2 (bing and ng only)", len(bing))
	}

	var tables int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'download_history'",
	).Scan(&tables); err != nil {
		t.Fatalf("checking for the dropped table: %v", err)
	}
	if tables != 0 {
		t.Error("download_history survived the migration")
	}
}

func TestMigration5MergesDuplicateURLs(t *testing.T) {
	db, path := openAtVersion(t, 4)

	for _, stmt := range []string{
		// Three applications of one picture: the oldest has the title, the
		// middle the thumbnail, the newest nothing but its time.
		`INSERT INTO wallpaper_history (id, channel_name, url, title, download_time, times_applied)
		 VALUES (1, 'ng', 'https://example.invalid/a.jpg', 'Title', 100, 1)`,
		`INSERT INTO wallpaper_history (id, channel_name, url, thumb_path, download_time, times_applied)
		 VALUES (2, 'ng', 'https://example.invalid/a.jpg', '/thumbs/a.jpg', 200, 2)`,
		`INSERT INTO wallpaper_history (id, channel_name, url, download_time)
		 VALUES (3, 'ng', 'https://example.invalid/a.jpg', 300)`,
		// A picture applied once is left alone.
		`INSERT INTO wallpaper_history (id, channel_name, url, download_time)
		 VALUES (4, 'bing', 'https://example.invalid/b.jpg', 150)`,
		// Entries with no URL cannot be told apart and are not merged.
		`INSERT INTO wallpaper_history (id, channel_name, url, download_time) VALUES (5, 'old', '', 50)`,
		`INSERT INTO wallpaper_history (id, channel_name, url, download_time) VALUES (6, 'old', '', 60)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seeding: %v\n%s", err, stmt)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	count, _ := s.WallpaperHistoryCount()
	if count != 4 {
		t.Fatalf("%d entries after merging, want 4 (a, b and the two without URLs)", count)
	}

	// Merged into the newest, which deduplication already compared against.
	got, err := s.WallpaperHistoryEntryByURL("https://example.invalid/a.jpg")
	if err != nil {
		t.Fatalf("WallpaperHistoryEntryByURL: %v", err)
	}
	if got.ID != 3 {
		t.Errorf("kept entry %d, want the newest, 3", got.ID)
	}
	if got.FirstAppliedTime != 100 || got.DownloadTime != 300 {
		t.Errorf("first/last = %d/%d, want 100/300", got.FirstAppliedTime, got.DownloadTime)
	}
	if got.TimesApplied != 4 {
		t.Errorf("TimesApplied = %d, want 1+2+1 = 4", got.TimesApplied)
	}
	// What the kept entry lacked is filled in from the ones merged into it.
	if got.Title != "Title" || got.ThumbPath != "/thumbs/a.jpg" {
		t.Errorf("title/thumb = %q/%q, want them carried over", got.Title, got.ThumbPath)
	}

	// From here on a duplicate is refused outright.
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "ng", URL: "https://example.invalid/a.jpg",
	}); err == nil {
		t.Error("a duplicate URL was accepted after the migration")
	}
}
