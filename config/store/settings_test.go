package store

import (
	"slices"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"github.com/genzj/goTApaper/channel"
)

// Options must satisfy the interface the download pipeline reads through.
var _ channel.Settings = (*Options)(nil)

func TestOptionsGetString(t *testing.T) {
	// Numbers reach these getters as float64 after a JSON round-trip, and as
	// int when a caller passes them in directly. Channels splice width and dpr
	// straight into query strings, so both have to stringify cleanly.
	o := NewOptions(map[string]any{
		"key":      "abc123",
		"width":    float64(1920),
		"height":   1080,
		"dpr":      int64(2),
		"featured": true,
	})

	cases := map[string]string{
		"key":      "abc123",
		"width":    "1920",
		"height":   "1080",
		"dpr":      "2",
		"featured": "true",
		"missing":  "",
	}
	for key, want := range cases {
		if got := o.GetString(key); got != want {
			t.Errorf("GetString(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestOptionsGetIntAndBool(t *testing.T) {
	o := NewOptions(map[string]any{
		"width":    float64(1920),
		"asString": "640",
		"featured": true,
		"flagText": "true",
		"zero":     float64(0),
	})

	if got := o.GetInt("width"); got != 1920 {
		t.Errorf("GetInt(width) = %d, want 1920", got)
	}
	if got := o.GetInt("asString"); got != 640 {
		t.Errorf("GetInt(asString) = %d, want 640", got)
	}
	if got := o.GetInt("missing"); got != 0 {
		t.Errorf("GetInt(missing) = %d, want 0", got)
	}
	if !o.GetBool("featured") || !o.GetBool("flagText") {
		t.Error("GetBool should accept both a real bool and its string spelling")
	}
	if o.GetBool("zero") || o.GetBool("missing") {
		t.Error("GetBool should be false for zero and for an absent key")
	}
}

// TestOptionsDottedLookup covers the keys channel/fix.go reads. It is the only
// channel that uses nested configuration, and it reads it with dotted keys.
func TestOptionsDottedLookup(t *testing.T) {
	o := NewOptions(map[string]any{
		"url": "file:///tmp/a.jpg",
		"meta": map[string]any{
			"title":       "Sunrise",
			"credit":      "Someone",
			"upload-time": "202401011200",
		},
	})

	if got := o.GetString("meta.title"); got != "Sunrise" {
		t.Errorf("GetString(meta.title) = %q, want Sunrise", got)
	}
	if got := o.GetString("meta.credit"); got != "Someone" {
		t.Errorf("GetString(meta.credit) = %q, want Someone", got)
	}
	if !o.IsSet("meta.upload-time") {
		t.Error("IsSet(meta.upload-time) = false, want true")
	}
	if o.IsSet("meta.caption") {
		t.Error("IsSet on an absent nested key should be false")
	}
	if got := o.GetString("meta.caption"); got != "" {
		t.Errorf("GetString(meta.caption) = %q, want empty", got)
	}
}

// TestOptionsFlatKeyBeatsNested pins the precedence that keeps the YAML import
// simple: a key stored literally as "meta.title" resolves without having to be
// restructured into a nested object first.
func TestOptionsFlatKeyBeatsNested(t *testing.T) {
	o := NewOptions(map[string]any{
		"meta.title": "flat",
		"meta":       map[string]any{"title": "nested"},
	})

	if got := o.GetString("meta.title"); got != "flat" {
		t.Errorf("GetString(meta.title) = %q, want the flat value", got)
	}
}

func TestOptionsGetStringMapString(t *testing.T) {
	// unsplash passes image_parameters through to the photo URL, so numeric
	// values have to come back as usable strings.
	o := NewOptions(map[string]any{
		"image_parameters": map[string]any{
			"q":   float64(100),
			"dpr": float64(2),
			"fm":  "jpg",
		},
	})

	got := o.GetStringMapString("image_parameters")
	want := map[string]string{"q": "100", "dpr": "2", "fm": "jpg"}
	if len(got) != len(want) {
		t.Fatalf("got %d parameters, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parameter %q = %q, want %q", k, got[k], v)
		}
	}

	if m := o.GetStringMapString("missing"); len(m) != 0 {
		t.Errorf("GetStringMapString on an absent key = %v, want empty", m)
	}
	if m := o.GetStringMapString("image_parameters.q"); len(m) != 0 {
		t.Errorf("GetStringMapString on a non-map = %v, want empty", m)
	}
}

func TestEffectiveChannelSettingsAppliesOverrides(t *testing.T) {
	c := EffectiveChannel{
		Name:    "bing",
		Type:    "bing-wallpaper",
		Options: map[string]any{"strategy": "largest-no-logo"},
	}

	// "force" is decided per run, not stored, so it arrives as an override
	// and must not be written back into the channel's options.
	s := c.Settings(map[string]any{"force": true})
	if !s.GetBool("force") {
		t.Error("override force=true was not visible")
	}
	if got := s.GetString("strategy"); got != "largest-no-logo" {
		t.Errorf("strategy = %q, want the stored value", got)
	}
	if _, leaked := c.Options["force"]; leaked {
		t.Error("an override leaked into the channel's stored options")
	}
}

// TestOptionsLogUnconvertibleValues checks that a mistyped option is reported.
//
// These getters cannot return an error, since channels read their settings as
// plain values. Without a log the only symptom of an option stored as the
// wrong type is a download quietly behaving as though it were unset.
func TestOptionsLogUnconvertibleValues(t *testing.T) {
	hook := logtest.NewLocal(logrus.StandardLogger())
	defer hook.Reset()

	previous := logrus.GetLevel()
	logrus.SetLevel(logrus.WarnLevel)
	defer logrus.SetLevel(previous)

	o := NewOptions(map[string]any{
		"width":            []any{1920},
		"image_parameters": "not-a-map",
		"featured":         []any{true},
	})

	if got := o.GetInt("width"); got != 0 {
		t.Errorf("GetInt on a list = %d, want 0", got)
	}
	if got := o.GetStringMapString("image_parameters"); len(got) != 0 {
		t.Errorf("GetStringMapString on a string = %v, want empty", got)
	}
	if o.GetBool("featured") {
		t.Error("GetBool on a list should be false")
	}

	var warned []string
	for _, e := range hook.AllEntries() {
		if e.Level == logrus.WarnLevel {
			if option, ok := e.Data["option"].(string); ok {
				warned = append(warned, option)
			}
			// The stored value may be an API key, so only its type is logged.
			if _, leaked := e.Data["value"]; leaked {
				t.Error("the option value was logged; only its type should be")
			}
		}
	}

	for _, want := range []string{"width", "image_parameters", "featured"} {
		if !slices.Contains(warned, want) {
			t.Errorf("no warning logged for %q; got %v", want, warned)
		}
	}
}
