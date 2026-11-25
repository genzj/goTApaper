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
		Name:      "Caption",
		Font:      "NotoSans-Regular.ttf",
		Size:      1.2,
		Color:     "222222",
		Anchor:    config.PositionBottomCenter,
		X:         0.5,
		Y:         0.9,
		Linespace: 1.4,
		Alignment: config.AlignCenter,
		Template:  "{{.Title}}",
		Background: store.WatermarkBackground{
			Color:       "eeeeee66",
			HThroughout: true,
			Padding:     [4]float64{0.2, 0.4, 0.2, 0.4},
		},
		Shadow:  store.WatermarkShadow{Color: "00000088", DX: 0.05, DY: 0.06},
		Outline: store.WatermarkOutline{Color: "ffffff", Width: 0.03},
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
	got.ID, want.SortOrder = 0, got.SortOrder
	if got != want {
		t.Errorf("round trip = %+v\nwant %+v", got, want)
	}
	if got.Template != want.Template {
		t.Errorf("template = %q, want %q", got.Template, want.Template)
	}
	if got.Background.Color != want.Background.Color || !got.Background.HThroughout {
		t.Errorf("background = %+v, want %+v", got.Background, want.Background)
	}
	if got.Background.Padding != want.Background.Padding {
		t.Errorf("padding = %v, want %v", got.Background.Padding, want.Background.Padding)
	}
}

// TestWatermarkLayerRejectsUnknownEnums covers the gap that moving these
// vocabularies into Go was meant to close. The renderer switches on these
// strings and silently falls back when it does not recognise one, so a typo
// used to produce a watermark in the wrong place with no diagnostic at all.
func TestWatermarkLayerRejectsUnknownEnums(t *testing.T) {
	s := openMemory(t)

	bad := sampleLayer()
	bad.Anchor = "middle" // not one of the nine valid positions
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
	updated.Anchor = config.PositionTopLeft
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
	if layers[0].Anchor != config.PositionTopLeft {
		t.Errorf("anchor = %q, want %q", layers[0].Anchor, config.PositionTopLeft)
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
