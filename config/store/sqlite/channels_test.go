package sqlite

import (
	"errors"
	"strings"
	"testing"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// unsplashTemplate mirrors the "unsplash-common-settings" anchor from
// examples/config.yaml.example: the shared API key and dimensions that two
// channel instances merge in.
func unsplashTemplate() store.ChannelTemplate {
	return store.ChannelTemplate{
		Name: "unsplash-common-settings",
		Type: "unsplash",
		Options: map[string]any{
			"key":         "shared-api-key",
			"strategy":    config.ByWidth,
			"width":       1920,
			"orientation": "landscape",
			"featured":    true,
		},
	}
}

func intptr(i int) *int { return &i }

func TestResolveLayersTemplateThenOptionsThenColumns(t *testing.T) {
	s := openMemory(t)

	if err := s.UpsertChannelTemplate(unsplashTemplate()); err != nil {
		t.Fatalf("UpsertChannelTemplate = %v", err)
	}

	err := s.UpsertChannel(store.ChannelInstance{
		Name:         "unsplash-kw-color",
		Type:         "unsplash",
		TemplateName: "unsplash-common-settings",
		Enabled:      true,
		Probability:  1,
		// Overrides the template's width via the promoted column.
		Width: intptr(2560),
		Options: map[string]any{
			"query": "color",
			// Overrides the template's orientation via instance options.
			"orientation": "portrait",
		},
	})
	if err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	eff, err := s.EffectiveChannel("unsplash-kw-color")
	if err != nil {
		t.Fatalf("EffectiveChannel = %v", err)
	}

	cases := []struct {
		key  string
		want any
		why  string
	}{
		{"key", "shared-api-key", "inherited from the template"},
		{"featured", true, "inherited from the template"},
		{"query", "color", "set only on the instance"},
		{"orientation", "portrait", "instance options beat the template"},
		{"width", 2560, "the promoted column beats both"},
		{"strategy", config.ByWidth, "inherited: the instance column is NULL"},
		{"type", "unsplash", "always the instance's registry type"},
	}
	for _, c := range cases {
		got, ok := eff.Options[c.key]
		if !ok {
			t.Errorf("option %q missing (%s)", c.key, c.why)
			continue
		}
		if !equalLoose(got, c.want) {
			t.Errorf("option %q = %v (%T), want %v (%s)", c.key, got, got, c.want, c.why)
		}
	}
}

// TestResolveInheritsWhenPromotedColumnIsNull pins down the behaviour that
// ruled out resolving this in SQL.
//
// The natural SQL spelling would overlay json_object('strategy', c.strategy,
// ...) onto the template with json_patch. But json_patch treats a null member
// as "remove this key", so a NULL strategy column would delete the template's
// strategy rather than inherit it -- the exact opposite of what NULL means
// here. Resolution therefore happens in Go, and this test is the regression
// guard for anyone tempted to push it back into the query.
func TestResolveInheritsWhenPromotedColumnIsNull(t *testing.T) {
	s := openMemory(t)

	if err := s.UpsertChannelTemplate(unsplashTemplate()); err != nil {
		t.Fatalf("UpsertChannelTemplate = %v", err)
	}
	err := s.UpsertChannel(store.ChannelInstance{
		Name:         "unsplash-kw-water",
		Type:         "unsplash",
		TemplateName: "unsplash-common-settings",
		Enabled:      true,
		Probability:  1,
		// Strategy, Width and Height all left nil: inherit everything.
		Options: map[string]any{"query": "water"},
	})
	if err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	eff, err := s.EffectiveChannel("unsplash-kw-water")
	if err != nil {
		t.Fatalf("EffectiveChannel = %v", err)
	}

	if got := eff.Options["strategy"]; !equalLoose(got, config.ByWidth) {
		t.Errorf("strategy = %v, want the template's %q; a nil column must inherit, not erase",
			got, config.ByWidth)
	}
	if got, ok := eff.Options["width"]; !ok || !equalLoose(got, 1920) {
		t.Errorf("width = %v (present=%v), want the template's 1920", got, ok)
	}
}

func TestEffectiveChannelsSkipsDisabledButByNameDoesNot(t *testing.T) {
	s := openMemory(t)

	// Disabling a channel should take it out of the automatic rotation while
	// leaving it runnable on explicit request, which is how the old
	// "in channels: but not in active-channels:" state behaved.
	c, err := s.Channel("ng")
	if err != nil {
		t.Fatalf("Channel = %v", err)
	}
	c.Enabled = false
	if err := s.UpsertChannel(c); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	eff, err := s.EffectiveChannels()
	if err != nil {
		t.Fatalf("EffectiveChannels = %v", err)
	}
	for _, e := range eff {
		if e.Name == "ng" {
			t.Error("EffectiveChannels included a disabled channel")
		}
	}

	if _, err := s.EffectiveChannel("ng"); err != nil {
		t.Errorf("EffectiveChannel(ng) = %v; a disabled channel must stay addressable", err)
	}
}

func TestUpsertChannelRejectsBadProbability(t *testing.T) {
	s := openMemory(t)

	err := s.UpsertChannel(store.ChannelInstance{
		Name: "oops", Type: "bing-wallpaper", Probability: 1.5,
	})
	if err == nil {
		t.Fatal("probability 1.5 should be rejected")
	}
}

func TestDeleteTemplateInUseIsRefused(t *testing.T) {
	s := openMemory(t)

	if err := s.UpsertChannelTemplate(unsplashTemplate()); err != nil {
		t.Fatalf("UpsertChannelTemplate = %v", err)
	}
	if err := s.UpsertChannel(store.ChannelInstance{
		Name: "unsplash-kw-color", Type: "unsplash",
		TemplateName: "unsplash-common-settings", Enabled: true, Probability: 1,
		Options: map[string]any{"query": "color"},
	}); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	err := s.DeleteChannelTemplate("unsplash-common-settings")
	if !errors.Is(err, store.ErrTemplateInUse) {
		t.Fatalf("DeleteChannelTemplate = %v, want ErrTemplateInUse", err)
	}
	// The error has to name the dependants, or the user cannot act on it.
	if !strings.Contains(err.Error(), "unsplash-kw-color") {
		t.Errorf("error %q should name the dependent channel", err)
	}

	// Crucially, the channel still has its inherited key.
	eff, err := s.EffectiveChannel("unsplash-kw-color")
	if err != nil {
		t.Fatalf("EffectiveChannel = %v", err)
	}
	if got := eff.Options["key"]; !equalLoose(got, "shared-api-key") {
		t.Errorf("after a refused delete, key = %v; the channel must be untouched", got)
	}
}

func TestInlineTemplatePreservesEffectiveConfig(t *testing.T) {
	s := openMemory(t)

	if err := s.UpsertChannelTemplate(unsplashTemplate()); err != nil {
		t.Fatalf("UpsertChannelTemplate = %v", err)
	}
	for _, name := range []string{"unsplash-kw-color", "unsplash-kw-water"} {
		if err := s.UpsertChannel(store.ChannelInstance{
			Name: name, Type: "unsplash",
			TemplateName: "unsplash-common-settings", Enabled: true, Probability: 1,
			Options: map[string]any{"query": strings.TrimPrefix(name, "unsplash-kw-")},
		}); err != nil {
			t.Fatalf("UpsertChannel(%s) = %v", name, err)
		}
	}

	before := map[string]map[string]any{}
	for _, name := range []string{"unsplash-kw-color", "unsplash-kw-water"} {
		eff, err := s.EffectiveChannel(name)
		if err != nil {
			t.Fatalf("EffectiveChannel(%s) = %v", name, err)
		}
		before[name] = eff.Options
	}

	if err := s.InlineChannelTemplate("unsplash-common-settings"); err != nil {
		t.Fatalf("InlineChannelTemplate = %v", err)
	}

	// The template is gone...
	if _, err := s.ChannelTemplate("unsplash-common-settings"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("template lookup after inlining = %v, want ErrNotFound", err)
	}
	// ...but nothing the channels actually resolve to has changed.
	for name, want := range before {
		eff, err := s.EffectiveChannel(name)
		if err != nil {
			t.Fatalf("EffectiveChannel(%s) = %v", name, err)
		}
		if eff.Options["key"] == nil {
			t.Errorf("%s lost its inherited key when the template was inlined", name)
		}
		for k, v := range want {
			if !equalLoose(eff.Options[k], v) {
				t.Errorf("%s option %q = %v after inlining, want %v", name, k, eff.Options[k], v)
			}
		}
	}
}

func TestReorderChannels(t *testing.T) {
	s := openMemory(t)

	if err := s.ReorderChannels([]string{"bing", "ng"}); err != nil {
		t.Fatalf("ReorderChannels = %v", err)
	}
	channels, err := s.Channels()
	if err != nil {
		t.Fatalf("Channels = %v", err)
	}
	if channels[0].Name != "bing" || channels[1].Name != "ng" {
		t.Errorf("order = %q, %q; want bing, ng", channels[0].Name, channels[1].Name)
	}

	// A partial list would leave channels sharing a position, making the
	// refresh order arbitrary, so it is refused.
	if err := s.ReorderChannels([]string{"bing"}); err == nil {
		t.Error("a partial reorder should be refused")
	}
	if err := s.ReorderChannels([]string{"bing", "nope"}); err == nil {
		t.Error("reordering an unknown channel should be refused")
	}
}

func TestDeleteChannelClearsHistory(t *testing.T) {
	s := openMemory(t)

	if err := s.PutHistory(store.HistoryEntry{
		ChannelName: "bing", URL: "https://example.invalid/a.jpg",
	}); err != nil {
		t.Fatalf("PutHistory = %v", err)
	}
	if err := s.DeleteChannel("bing"); err != nil {
		t.Fatalf("DeleteChannel = %v", err)
	}

	// A stale history row would suppress the first download of a channel
	// recreated later under the same name.
	if _, err := s.History("bing"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("History after DeleteChannel = %v, want ErrNotFound", err)
	}
}

func TestTxRollsBackOnError(t *testing.T) {
	s := openMemory(t)

	sentinel := errors.New("boom")
	err := s.Tx(func(tx store.Store) error {
		if err := tx.SetSetting(config.KeyNetworkProxy, "socks5://127.0.0.1:1080"); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Tx = %v, want the sentinel", err)
	}

	got, err := s.Setting(config.KeyNetworkProxy)
	if err != nil {
		t.Fatalf("Setting = %v", err)
	}
	if !got.IsDefault {
		t.Errorf("proxy = %+v after a rolled-back Tx; want the default", got)
	}
}

// equalLoose compares values that may have round-tripped through JSON, where
// every number becomes a float64.
func equalLoose(got, want any) bool {
	if got == nil || want == nil {
		return got == want
	}
	if gs, ok := asString(got); ok {
		if ws, ok := asString(want); ok {
			return gs == ws
		}
	}
	return got == want
}
