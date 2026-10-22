package sqlite

import (
	"errors"
	"testing"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

func sampleLayer() store.WatermarkLayer {
	return store.WatermarkLayer{
		SortOrder: store.AppendOrder,
		Enabled:   true,
		Font:      "NotoSans-Regular.ttf",
		Point:     13,
		Color:     "222222",
		Position:  config.PositionBottomCenter,
		HMargin:   0.1,
		VMargin:   0.1,
		Linespace: 1.4,
		Alignment: config.AlignCenter,
		Template:  "{{.Title}}",
		Background: store.WatermarkBackground{
			Color:       "eeeeee66",
			HThroughout: true,
			Paddings:    []float64{0.2},
		},
	}
}

func TestWatermarkLayerRoundTrip(t *testing.T) {
	s := openMemory(t)

	if err := s.UpsertWatermarkLayer(sampleLayer()); err != nil {
		t.Fatalf("UpsertWatermarkLayer = %v", err)
	}

	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(layers))
	}

	got := layers[0]
	want := sampleLayer()
	if got.Position != want.Position || got.Alignment != want.Alignment {
		t.Errorf("position/alignment = %q/%q, want %q/%q",
			got.Position, got.Alignment, want.Position, want.Alignment)
	}
	if got.Template != want.Template {
		t.Errorf("template = %q, want %q", got.Template, want.Template)
	}
	if got.Background.Color != want.Background.Color || !got.Background.HThroughout {
		t.Errorf("background = %+v, want %+v", got.Background, want.Background)
	}
	if len(got.Background.Paddings) != 1 || got.Background.Paddings[0] != 0.2 {
		t.Errorf("paddings = %v, want [0.2]", got.Background.Paddings)
	}
}

// TestWatermarkLayerRejectsUnknownEnums covers the gap that moving these
// vocabularies into Go was meant to close. The renderer switches on these
// strings and silently falls back when it does not recognise one, so a typo
// used to produce a watermark in the wrong place with no diagnostic at all.
func TestWatermarkLayerRejectsUnknownEnums(t *testing.T) {
	s := openMemory(t)

	bad := sampleLayer()
	bad.Position = "middle" // not one of the nine valid positions
	err := s.UpsertWatermarkLayer(bad)
	var invalid *config.InvalidValueError
	if !errors.As(err, &invalid) {
		t.Fatalf("position \"middle\" = %v, want InvalidValueError", err)
	}
	if len(invalid.Expected) != len(config.PositionValues) {
		t.Errorf("error should list all %d positions, listed %d",
			len(config.PositionValues), len(invalid.Expected))
	}

	bad = sampleLayer()
	bad.Alignment = "justified"
	if err := s.UpsertWatermarkLayer(bad); !errors.As(err, &invalid) {
		t.Fatalf("alignment \"justified\" = %v, want InvalidValueError", err)
	}

	// Neither bad layer should have been stored.
	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}
	if len(layers) != 0 {
		t.Errorf("rejected layers were persisted anyway: %+v", layers)
	}
}

func TestWatermarkLayerUpdateAndDelete(t *testing.T) {
	s := openMemory(t)

	if err := s.UpsertWatermarkLayer(sampleLayer()); err != nil {
		t.Fatalf("UpsertWatermarkLayer = %v", err)
	}
	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}

	updated := layers[0]
	updated.Position = config.PositionTopLeft
	if err := s.UpsertWatermarkLayer(updated); err != nil {
		t.Fatalf("update = %v", err)
	}

	layers, err = s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("update created a second row: got %d layers", len(layers))
	}
	if layers[0].Position != config.PositionTopLeft {
		t.Errorf("position = %q, want %q", layers[0].Position, config.PositionTopLeft)
	}

	if err := s.DeleteWatermarkLayer(updated.ID); err != nil {
		t.Fatalf("DeleteWatermarkLayer = %v", err)
	}
	if err := s.DeleteWatermarkLayer(updated.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}

func TestWatermarkLayersAppendInOrder(t *testing.T) {
	s := openMemory(t)

	first := sampleLayer()
	first.Template = "first"
	second := sampleLayer()
	second.Template = "second"

	if err := s.UpsertWatermarkLayer(first); err != nil {
		t.Fatalf("first insert = %v", err)
	}
	if err := s.UpsertWatermarkLayer(second); err != nil {
		t.Fatalf("second insert = %v", err)
	}

	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers = %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(layers))
	}
	// A new layer appends rather than landing on top of the existing one.
	if layers[0].Template != "first" || layers[1].Template != "second" {
		t.Errorf("order = %q, %q; want first, second",
			layers[0].Template, layers[1].Template)
	}
}

func TestPutHistoryOverwritesPerChannel(t *testing.T) {
	s := openMemory(t)

	// One row per channel, exactly like the history.json map it replaces: a
	// second download for the same channel replaces the first rather than
	// appending.
	for _, url := range []string{"https://example.invalid/1.jpg", "https://example.invalid/2.jpg"} {
		if err := s.PutHistory(store.HistoryEntry{
			ChannelName: "bing", URL: url, Title: "t",
		}); err != nil {
			t.Fatalf("PutHistory(%s) = %v", url, err)
		}
	}

	entries, err := s.Histories()
	if err != nil {
		t.Fatalf("Histories = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d history rows for one channel, want 1", len(entries))
	}
	if entries[0].URL != "https://example.invalid/2.jpg" {
		t.Errorf("url = %q, want the most recent", entries[0].URL)
	}
	if entries[0].DownloadTime == 0 {
		t.Error("download time should be stamped when the caller omits it")
	}
}
