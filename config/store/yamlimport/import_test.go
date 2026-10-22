package yamlimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/config/store/sqlite"
)

func init() {
	config.SetAppName("goTApaper")
}

func openMemory(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(sqlite.MemoryPath)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// importDoc imports a document into a fresh store, clearing the channels the
// migration seeds so assertions see only what the document defined.
func importDoc(t *testing.T, doc string) *sqlite.Store {
	t.Helper()
	s := openMemory(t)
	for _, name := range []string{"ng", "bing"} {
		if err := s.DeleteChannel(name); err != nil {
			t.Fatalf("clearing seeded channel %s = %v", name, err)
		}
	}
	if err := Import(s, []byte(doc)); err != nil {
		t.Fatalf("Import = %v", err)
	}
	return s
}

// TestImportExampleConfig runs the shipped example through the importer. It is
// the broadest guard available: the example exercises anchors, merge keys, both
// active-channels spellings, and a full watermark layer.
func TestImportExampleConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "config.yaml.example"))
	if err != nil {
		t.Fatalf("cannot read the example config: %v", err)
	}

	s := openMemory(t)
	if err := Import(s, data); err != nil {
		t.Fatalf("Import = %v", err)
	}

	// Both shared-settings blocks must survive as templates. Flattening them
	// would copy the user's API key into every channel and make editing it
	// later an N-place change -- the regression this design exists to prevent.
	templates, err := s.ChannelTemplates()
	if err != nil {
		t.Fatalf("ChannelTemplates = %v", err)
	}
	byName := map[string]store.ChannelTemplate{}
	for _, tpl := range templates {
		byName[tpl.Name] = tpl
	}
	for _, want := range []string{"pexels-common-settings", "unsplash-common-settings"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("template %q was not imported; got %v", want, templates)
		}
	}

	if got := byName["unsplash-common-settings"].Options["key"]; got != "unsplash-api-key-id" {
		t.Errorf("template key = %v, want the value from the anchor", got)
	}

	// The two unsplash channels must *reference* the template, not carry
	// their own copies of its settings.
	for _, name := range []string{"unsplash-kw-color", "unsplash-kw-water"} {
		c, err := s.Channel(name)
		if err != nil {
			t.Fatalf("Channel(%s) = %v", name, err)
		}
		if c.TemplateName != "unsplash-common-settings" {
			t.Errorf("%s template = %q, want unsplash-common-settings", name, c.TemplateName)
		}
		if _, duplicated := c.Options["key"]; duplicated {
			t.Errorf("%s has its own copy of the API key; the anchor was flattened", name)
		}

		// ...but must still resolve to it.
		eff, err := s.EffectiveChannel(name)
		if err != nil {
			t.Fatalf("EffectiveChannel(%s) = %v", name, err)
		}
		if eff.Options["key"] != "unsplash-api-key-id" {
			t.Errorf("%s effective key = %v, want the inherited value", name, eff.Options["key"])
		}
		if eff.Options["orientation"] != "landscape" {
			t.Errorf("%s did not inherit orientation: %v", name, eff.Options["orientation"])
		}
	}

	// The watermark layer should arrive intact.
	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("got %d watermark layers, want 1", len(layers))
	}
	if layers[0].Position != config.PositionBottomCenter {
		t.Errorf("watermark position = %q, want bottom-center", layers[0].Position)
	}
	if layers[0].Background.Color != "eeeeee66" {
		t.Errorf("background color = %q, want eeeeee66", layers[0].Background.Color)
	}
	if layers[0].Template == "" {
		t.Error("watermark template was not imported")
	}
}

// TestImportPreservesActiveChannelOrder guards a bug that slipped through
// once: sort order zero is a real position, and treating it as "unset" moved
// whatever the user listed first to the very end of the rotation.
func TestImportPreservesActiveChannelOrder(t *testing.T) {
	s := importDoc(t, `
active-channels:
  - first
  - second: 0.5
  - third
channels:
  first:
    type: bing-wallpaper
  second:
    type: unsplash
    key: k
  third:
    type: ng-photo-of-today
  never:
    type: bing-wallpaper
`)

	channels, err := s.Channels()
	if err != nil {
		t.Fatalf("Channels = %v", err)
	}

	var gotOrder []string
	for _, c := range channels {
		gotOrder = append(gotOrder, c.Name)
	}
	want := []string{"first", "second", "third", "never"}
	if len(gotOrder) != len(want) {
		t.Fatalf("channels = %v, want %v", gotOrder, want)
	}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Errorf("channel order = %v, want %v", gotOrder, want)
			break
		}
	}

	// A channel defined but never activated imports disabled rather than
	// being dropped, so the user does not lose a definition.
	never, err := s.Channel("never")
	if err != nil {
		t.Fatalf("Channel(never) = %v", err)
	}
	if never.Enabled {
		t.Error("a channel absent from active-channels should import disabled")
	}

	// The map form carries a probability.
	second, err := s.Channel("second")
	if err != nil {
		t.Fatalf("Channel(second) = %v", err)
	}
	if second.Probability != 0.5 {
		t.Errorf("second probability = %v, want 0.5", second.Probability)
	}
	if !second.Enabled {
		t.Error("second should be enabled")
	}
}

func TestImportGlobalsSkipDefaults(t *testing.T) {
	s := importDoc(t, `
crop: "no"
proxy: direct
daemon:
  interval: 60
channels:
  only:
    type: bing-wallpaper
active-channels:
  - only
`)

	// A non-default value is stored.
	crop, err := s.Setting(config.KeyPictureCrop)
	if err != nil {
		t.Fatalf("Setting(crop) = %v", err)
	}
	if crop.Value != config.CropNo || crop.IsDefault {
		t.Errorf("crop = %+v, want the stored value %q", crop, config.CropNo)
	}

	// A value identical to the default is not stored: freezing it would stop
	// a future change to that default from reaching this user.
	proxy, err := s.Setting(config.KeyNetworkProxy)
	if err != nil {
		t.Fatalf("Setting(proxy) = %v", err)
	}
	if !proxy.IsDefault {
		t.Error("a value equal to the default should not be stored")
	}
}

// TestImportToleratesBadValues pins the decision that one unusable line must
// not stop the application from starting.
func TestImportToleratesBadValues(t *testing.T) {
	s := importDoc(t, `
crop: maybe
not-a-real-setting: 1
reference-width: 2560
watermark:
  - position: nowhere
    alignment: sideways
    template: "{{.Title}}"
channels:
  only:
    type: bing-wallpaper
active-channels:
  - only
`)

	// The invalid enum is ignored and left at its default...
	crop, err := s.Setting(config.KeyPictureCrop)
	if err != nil {
		t.Fatalf("Setting(crop) = %v", err)
	}
	if !crop.IsDefault {
		t.Errorf("crop = %+v, want the default after an invalid value", crop)
	}

	// ...while the valid settings around it still import.
	width, err := s.Setting(config.KeyPictureReferenceWidth)
	if err != nil {
		t.Fatalf("Setting(reference-width) = %v", err)
	}
	if width.Value != "2560" {
		t.Errorf("reference-width = %q, want 2560", width.Value)
	}

	// An unrecognised watermark position is corrected rather than rejected,
	// since the store would otherwise refuse the row and abort the import.
	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(layers))
	}
	if !config.ValidPosition(layers[0].Position) {
		t.Errorf("position = %q, want a valid one", layers[0].Position)
	}
	if !config.ValidAlignment(layers[0].Alignment) {
		t.Errorf("alignment = %q, want a valid one", layers[0].Alignment)
	}
}

func TestImportIfPresentArchivesFiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	historyPath := filepath.Join(dir, "history.json")

	if err := os.WriteFile(cfgPath, []byte(`
channels:
  bing:
    type: bing-wallpaper
active-channels:
  - bing
`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyPath, []byte(
		`{"Meta":{},"History":{"bing":{"Name":"bing","History":"https://example.invalid/a.jpg"}}}`,
	), 0600); err != nil {
		t.Fatal(err)
	}

	s := openMemory(t)
	if err := ImportIfPresent(s, cfgPath); err != nil {
		t.Fatalf("ImportIfPresent = %v", err)
	}

	// Both files are renamed, not deleted: the database is now authoritative,
	// but the originals stay recoverable.
	for _, original := range []string{cfgPath, historyPath} {
		if _, err := os.Stat(original); !os.IsNotExist(err) {
			t.Errorf("%s should have been renamed away", original)
		}
		if _, err := os.Stat(original + ".migrated"); err != nil {
			t.Errorf("%s.migrated is missing: %v", original, err)
		}
	}

	// History transfers one row per channel, matching the old file's shape.
	h, err := s.History("bing")
	if err != nil {
		t.Fatalf("History = %v", err)
	}
	if h.URL != "https://example.invalid/a.jpg" {
		t.Errorf("history URL = %q, want the imported one", h.URL)
	}
}

func TestImportIfPresentMissingFileIsNotAnError(t *testing.T) {
	s := openMemory(t)
	if err := ImportIfPresent(s, filepath.Join(t.TempDir(), "absent.yaml")); err != nil {
		t.Errorf("ImportIfPresent on a missing file = %v, want nil", err)
	}
}

// TestExportRoundTrip is the guarantee that makes the database safe to adopt:
// a configuration can leave as YAML and come back without losing anything,
// including the template sharing that anchors express.
func TestExportRoundTrip(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "config.yaml.example"))
	if err != nil {
		t.Fatalf("cannot read the example config: %v", err)
	}

	first := openMemory(t)
	if err := Import(first, data); err != nil {
		t.Fatalf("initial Import = %v", err)
	}

	exported, err := Export(first)
	if err != nil {
		t.Fatalf("Export = %v", err)
	}

	// The exported document must carry anchors and merge keys, not flattened
	// copies, or the next person to edit the API key has to do it N times.
	text := string(exported)
	for _, want := range []string{"&unsplash-common-settings", "<<: *unsplash-common-settings"} {
		if !strings.Contains(text, want) {
			t.Errorf("exported YAML is missing %q:\n%s", want, text)
		}
	}

	second := openMemory(t)
	if err := Import(second, exported); err != nil {
		t.Fatalf("re-import of exported YAML = %v", err)
	}

	// Re-exporting must be byte-identical: that is what proves nothing was
	// lost or invented along the way.
	reExported, err := Export(second)
	if err != nil {
		t.Fatalf("second Export = %v", err)
	}
	if string(reExported) != text {
		t.Errorf("round trip is not stable.\nfirst:\n%s\nsecond:\n%s", text, reExported)
	}
}

// TestImportNestedAndLegacyKeys covers both halves of the key mapping.
//
// The old format spells a namespaced setting as a nested mapping
// (`daemon:` / `  interval: 60`), and spells the rest with flat dashed names
// that the store has since namespaced. An earlier revision skipped every
// non-scalar top-level key, which silently dropped a customised refresh
// interval — the user's value simply vanished with no warning.
func TestImportNestedAndLegacyKeys(t *testing.T) {
	s := importDoc(t, `
daemon:
  interval: 60
api:
  port: 7777
crop: "no"
reference-width: 2560
debug: true
proxy: socks5://127.0.0.1:1080
channels:
  only:
    type: bing-wallpaper
active-channels:
  - only
`)

	for _, tc := range []struct {
		key, want string
	}{
		// Nested mappings become dotted keys.
		{config.KeyDaemonInterval, "60"},
		{config.KeyAPIPort, "7777"},
		// Flat legacy names map onto their namespaced equivalents.
		{config.KeyPictureCrop, config.CropNo},
		{config.KeyPictureReferenceWidth, "2560"},
		{config.KeyLogDebug, "true"},
		{config.KeyNetworkProxy, "socks5://127.0.0.1:1080"},
	} {
		got, err := s.Setting(tc.key)
		if err != nil {
			t.Errorf("Setting(%s) = %v", tc.key, err)
			continue
		}
		if got.Value != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got.Value, tc.want)
		}
		if got.IsDefault {
			t.Errorf("%s was not stored; the imported value was dropped", tc.key)
		}
	}
}

// TestImportExampleKeepsDaemonInterval is the narrower regression guard: the
// shipped example sets the interval to its default value, which is why the
// dropped-nested-key bug went unnoticed at first.
func TestImportExampleKeepsDaemonInterval(t *testing.T) {
	s := importDoc(t, `
daemon:
  interval: 3600
channels:
  only:
    type: bing-wallpaper
active-channels:
  - only
`)

	got, err := s.Setting(config.KeyDaemonInterval)
	if err != nil {
		t.Fatalf("Setting = %v", err)
	}
	// Equal to the default, so correctly not stored -- but it must still read
	// back as the right value rather than as zero.
	if got.Value != "3600" {
		t.Errorf("daemon.interval = %q, want 3600", got.Value)
	}
}

// TestExportNestsNamespacedKeys checks the export side of the key notation:
// namespaced keys come back out as nested mappings, and two keys sharing a
// namespace land under one parent rather than in two separate blocks.
func TestExportNestsNamespacedKeys(t *testing.T) {
	s := openMemory(t)

	for key, value := range map[string]string{
		config.KeyDaemonInterval:         "60",
		config.KeyAPIPort:                "7777",
		config.KeyPictureCrop:            config.CropNo,
		config.KeyPictureReferenceWidth:  "2560",
		config.KeyPictureReferenceHeight: "1440",
	} {
		if err := s.SetSetting(key, value); err != nil {
			t.Fatalf("SetSetting(%s) = %v", key, err)
		}
	}

	exported, err := Export(s)
	if err != nil {
		t.Fatalf("Export = %v", err)
	}
	text := string(exported)

	// One "picture:" block holding both reference dimensions and the crop
	// mode, not three top-level keys and not three separate blocks.
	if n := strings.Count(text, "picture:"); n != 1 {
		t.Errorf("found %d \"picture:\" blocks, want exactly 1:\n%s", n, text)
	}
	for _, want := range []string{"interval: 60", "port: 7777", "reference-width: 2560"} {
		if !strings.Contains(text, want) {
			t.Errorf("exported YAML is missing %q:\n%s", want, text)
		}
	}

	// And it must survive a round trip.
	second := openMemory(t)
	if err := Import(second, exported); err != nil {
		t.Fatalf("re-import = %v", err)
	}
	for key, want := range map[string]string{
		config.KeyDaemonInterval:        "60",
		config.KeyAPIPort:               "7777",
		config.KeyPictureReferenceWidth: "2560",
	} {
		got, err := second.Setting(key)
		if err != nil {
			t.Fatalf("Setting(%s) = %v", key, err)
		}
		if got.Value != want {
			t.Errorf("%s survived export as %q, want %q", key, got.Value, want)
		}
	}
}
