package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

func TestContentSchemaStampedOnCreate(t *testing.T) {
	s := openMemory(t)

	got, err := s.SchemaVersion(MetaSettingsSchema)
	if err != nil {
		t.Fatalf("SchemaVersion = %v", err)
	}
	if got != config.SettingsSchemaVersion {
		t.Errorf("settings schema version = %d, want %d", got, config.SettingsSchemaVersion)
	}
}

// TestContentSchemaRefusesNewer is the point of versioning the content model
// separately from the table structure.
//
// A newer build may rename or re-mean a setting key. An older build opening
// that database would read the renamed key as absent and fall back to a
// default, then run with a configuration the user never chose. Refusing is the
// safe outcome.
func TestContentSchemaRefusesNewer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gotapaper.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	if err := s.SetSchemaVersion(MetaSettingsSchema, config.SettingsSchemaVersion+1); err != nil {
		t.Fatalf("SetSchemaVersion = %v", err)
	}
	s.Close()

	if _, err := Open(path); !errors.Is(err, store.ErrSchemaTooNew) {
		t.Fatalf("Open on a newer content schema = %v, want ErrSchemaTooNew", err)
	}
}

// TestContentSchemaUpgradesOlder covers the other direction: an older content
// version is adopted rather than refused, because this build understands it.
func TestContentSchemaUpgradesOlder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gotapaper.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	if err := s.SetSchemaVersion(MetaSettingsSchema, 0); err != nil {
		t.Fatalf("SetSchemaVersion = %v", err)
	}
	s.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open on an older content schema = %v, want success", err)
	}
	defer reopened.Close()

	got, err := reopened.SchemaVersion(MetaSettingsSchema)
	if err != nil {
		t.Fatalf("SchemaVersion = %v", err)
	}
	if got != config.SettingsSchemaVersion {
		t.Errorf("version after upgrade = %d, want %d", got, config.SettingsSchemaVersion)
	}
}

// TestChannelOptionsVersionRecorded checks every written instance carries the
// option-schema version of its type, which is what lets a channel that has
// changed its option shape recognise an older row.
func TestChannelOptionsVersionRecorded(t *testing.T) {
	s := openMemory(t)

	// The seeded channels are written by the migration, which stamps 1.
	bing, err := s.Channel("bing")
	if err != nil {
		t.Fatalf("Channel = %v", err)
	}
	if bing.OptionsVersion != 1 {
		t.Errorf("seeded options version = %d, want 1", bing.OptionsVersion)
	}

	// A channel written through the store picks up its type's live version.
	if err := s.UpsertChannel(store.ChannelInstance{
		Name: "fresh", Type: "bing-wallpaper", Enabled: true, Probability: 1,
		SortOrder: store.AppendOrder,
	}); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}
	fresh, err := s.Channel("fresh")
	if err != nil {
		t.Fatalf("Channel = %v", err)
	}
	if fresh.OptionsVersion < 1 {
		t.Errorf("options version = %d, want at least 1", fresh.OptionsVersion)
	}

	// An unknown type still gets a usable version rather than zero, so a row
	// for a channel this build no longer ships is not mistaken for unversioned.
	if err := s.UpsertChannel(store.ChannelInstance{
		Name: "gone", Type: "no-such-channel-type", Enabled: false, Probability: 1,
		SortOrder: store.AppendOrder,
	}); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}
	gone, err := s.Channel("gone")
	if err != nil {
		t.Fatalf("Channel = %v", err)
	}
	if gone.OptionsVersion != 1 {
		t.Errorf("unknown type options version = %d, want 1", gone.OptionsVersion)
	}
}
