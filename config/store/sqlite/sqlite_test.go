package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

func init() {
	// AppDir panics without an app name, and the settings schema reaches for
	// DefaultSetter, so give the config package the identity it expects.
	config.SetAppName("goTApaper")
}

func openMemory(t *testing.T) *Store {
	t.Helper()
	s, err := Open(MemoryPath)
	if err != nil {
		t.Fatalf("Open(MemoryPath) = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesSchemaAndSeeds(t *testing.T) {
	s := openMemory(t)

	channels, err := s.Channels()
	if err != nil {
		t.Fatalf("Channels() = %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 seeded channels, got %d: %+v", len(channels), channels)
	}

	// Seed order matters: it is the refresh order, and "bing" is last because
	// it reliably returns a picture.
	if channels[0].Name != "ng" || channels[1].Name != "bing" {
		t.Errorf("seeded channel order = %q, %q; want ng, bing",
			channels[0].Name, channels[1].Name)
	}
	if got := channels[1].Type; got != "bing-wallpaper" {
		t.Errorf("bing type = %q, want bing-wallpaper", got)
	}
	if channels[1].Strategy == nil || *channels[1].Strategy != config.LargestNoLogo {
		t.Errorf("bing strategy = %v, want %q", channels[1].Strategy, config.LargestNoLogo)
	}
	if !channels[0].Enabled {
		t.Error("seeded channels should be enabled")
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	// ON DELETE RESTRICT is the only thing standing between a template delete
	// and a silently broken channel, and SQLite ignores it unless the pragma
	// is on. Verify the pragma rather than trusting it.
	s := openMemory(t)

	var on int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatalf("PRAGMA foreign_keys = %v", err)
	}
	if on != 1 {
		t.Error("foreign_keys pragma is off; ON DELETE RESTRICT will not be enforced")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gotapaper.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open = %v", err)
	}
	if err := first.SetSetting(config.KeyNetworkProxy, "socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("SetSetting = %v", err)
	}
	first.Close()

	// Reopening must not re-run migrations, which would re-insert the seed
	// rows and resurrect defaults over the user's choices.
	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open = %v", err)
	}
	defer second.Close()

	got, err := second.Setting(config.KeyNetworkProxy)
	if err != nil {
		t.Fatalf("Setting = %v", err)
	}
	if got.Value != "socks5://127.0.0.1:1080" {
		t.Errorf("proxy survived reopen as %q, want the stored value", got.Value)
	}

	channels, err := second.Channels()
	if err != nil {
		t.Fatalf("Channels = %v", err)
	}
	if len(channels) != 2 {
		t.Errorf("reopening duplicated seed rows: got %d channels, want 2", len(channels))
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gotapaper.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatalf("bumping user_version = %v", err)
	}
	s.Close()

	// An older binary opening a newer database must refuse rather than
	// operate on a schema it does not understand.
	_, err = Open(path)
	if !errors.Is(err, store.ErrSchemaTooNew) {
		t.Fatalf("Open on a v99 database = %v, want ErrSchemaTooNew", err)
	}
}

func TestSettingFallsBackToSchemaDefault(t *testing.T) {
	s := openMemory(t)

	got, err := s.Setting(config.KeyPictureCrop)
	if err != nil {
		t.Fatalf("Setting = %v", err)
	}
	if !got.IsDefault {
		t.Error("an unset key should report IsDefault")
	}
	if got.Value != config.CropYes {
		t.Errorf("default crop = %q, want %q", got.Value, config.CropYes)
	}
}

func TestSetSettingRoundTripAndUnset(t *testing.T) {
	s := openMemory(t)

	if err := s.SetSetting(config.KeyPictureCrop, config.CropWinOnly); err != nil {
		t.Fatalf("SetSetting = %v", err)
	}
	got, err := s.Setting(config.KeyPictureCrop)
	if err != nil {
		t.Fatalf("Setting = %v", err)
	}
	if got.Value != config.CropWinOnly || got.IsDefault {
		t.Errorf("after set, crop = %+v; want %q and IsDefault false", got, config.CropWinOnly)
	}

	if err := s.UnsetSetting(config.KeyPictureCrop); err != nil {
		t.Fatalf("UnsetSetting = %v", err)
	}
	got, err = s.Setting(config.KeyPictureCrop)
	if err != nil {
		t.Fatalf("Setting = %v", err)
	}
	if !got.IsDefault || got.Value != config.CropYes {
		t.Errorf("after unset, crop = %+v; want the default back", got)
	}
}

func TestSetSettingRejectsInvalidValues(t *testing.T) {
	s := openMemory(t)

	// The point of keeping the enum in Go: an invalid value is rejected with
	// the acceptable set, and widening it needs no migration.
	err := s.SetSetting(config.KeyPictureCrop, "maybe")
	var invalid *config.InvalidValueError
	if !errors.As(err, &invalid) {
		t.Fatalf("SetSetting(crop, maybe) = %v, want InvalidValueError", err)
	}
	if len(invalid.Expected) == 0 {
		t.Error("InvalidValueError should list the acceptable values")
	}

	if err := s.SetSetting(config.KeyDaemonInterval, "soon"); err == nil {
		t.Error("SetSetting(daemon.interval, soon) should reject a non-integer")
	}
	if err := s.SetSetting("no-such-key", "x"); !errors.Is(err, config.ErrUnknownKey) {
		t.Errorf("SetSetting on an unknown key = %v, want ErrUnknownKey", err)
	}
}

func TestSettingsIgnoresUnknownStoredKeys(t *testing.T) {
	s := openMemory(t)

	// Simulate a database written by a newer build. The older binary should
	// skip the key it does not recognise rather than fail to start.
	if _, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES ('future-feature', 'on')",
	); err != nil {
		t.Fatalf("seeding unknown key = %v", err)
	}

	settings, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings = %v", err)
	}
	for _, st := range settings {
		if st.Key == "future-feature" {
			t.Error("Settings surfaced a key absent from the schema")
		}
	}
	if len(settings) != len(config.Definitions()) {
		t.Errorf("Settings returned %d entries, want one per definition (%d)",
			len(settings), len(config.Definitions()))
	}
}

func TestSetSettingEmitsChangeEvent(t *testing.T) {
	s := openMemory(t)

	// The daemon subscribes to "*" to refresh immediately on a config change;
	// without this event it would sit out the rest of its sleep interval.
	seen := make(chan string, 1)
	config.Observe(config.KeyDaemonInterval, func(key string, _, _ any) {
		select {
		case seen <- key:
		default:
		}
	})

	if err := s.SetSetting(config.KeyDaemonInterval, "60"); err != nil {
		t.Fatalf("SetSetting = %v", err)
	}

	if got := <-seen; got != config.KeyDaemonInterval {
		t.Errorf("observed key = %q, want %q", got, config.KeyDaemonInterval)
	}
}

func TestBackupTakenBeforeMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gotapaper.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	s.Close()

	// Pretend the database predates the current schema, so reopening has to
	// migrate it and therefore has to back it up first.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen = %v", err)
	}
	if _, err := reopened.db.Exec("PRAGMA user_version = 0"); err != nil {
		t.Fatalf("resetting user_version = %v", err)
	}
	reopened.Close()

	// Migrating from 0 re-runs 0001, which conflicts with the existing tables;
	// what matters here is that the backup was written before that was tried.
	Open(path)

	matches, err := filepath.Glob(path + ".pre-v*.bak")
	if err != nil {
		t.Fatalf("Glob = %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no pre-migration backup was written")
	}
	if info, err := os.Stat(matches[0]); err != nil || info.Size() == 0 {
		t.Errorf("backup %s is missing or empty", matches[0])
	}
}
