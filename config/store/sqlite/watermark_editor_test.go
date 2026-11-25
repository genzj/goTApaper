package sqlite

import (
	"errors"
	"math"
	"testing"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

func TestReplaceWatermarkLayersKeepsReordersAndDeletes(t *testing.T) {
	s := openMemory(t)

	for _, tmpl := range []string{"a", "b", "c"} {
		l := sampleLayer()
		l.Template = tmpl
		if err := s.UpsertWatermarkLayer(l); err != nil {
			t.Fatalf("seed %s: %v", tmpl, err)
		}
	}
	before, _ := s.WatermarkLayers()
	a, c := before[0], before[2]

	fresh := sampleLayer()
	fresh.Template = "new"
	// c first, then a new layer, then a; b is dropped.
	got, err := s.ReplaceWatermarkLayers([]store.WatermarkLayer{c, fresh, a})
	if err != nil {
		t.Fatalf("ReplaceWatermarkLayers = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d layers, want 3", len(got))
	}
	for i, want := range []string{"c", "new", "a"} {
		if got[i].Template != want {
			t.Errorf("layer %d = %q, want %q", i, got[i].Template, want)
		}
	}
	if got[0].ID != c.ID || got[2].ID != a.ID {
		t.Errorf("existing layers lost their ids: %d,%d want %d,%d", got[0].ID, got[2].ID, c.ID, a.ID)
	}
}

func TestReplaceWatermarkLayersIsAtomic(t *testing.T) {
	s := openMemory(t)
	if err := s.UpsertWatermarkLayer(sampleLayer()); err != nil {
		t.Fatal(err)
	}

	bad := sampleLayer()
	bad.Color = "not-a-colour"
	_, err := s.ReplaceWatermarkLayers([]store.WatermarkLayer{sampleLayer(), bad})
	var invalid *config.InvalidValueError
	if !errors.As(err, &invalid) {
		t.Fatalf("ReplaceWatermarkLayers = %v, want InvalidValueError", err)
	}

	layers, _ := s.WatermarkLayers()
	if len(layers) != 1 {
		t.Errorf("a rejected replace changed the list: %d layers", len(layers))
	}
}

func TestReplaceWatermarkLayersDuplicateIDBecomesNewLayer(t *testing.T) {
	s := openMemory(t)
	if err := s.UpsertWatermarkLayer(sampleLayer()); err != nil {
		t.Fatal(err)
	}
	layers, _ := s.WatermarkLayers()

	got, err := s.ReplaceWatermarkLayers([]store.WatermarkLayer{layers[0], layers[0]})
	if err != nil {
		t.Fatalf("ReplaceWatermarkLayers = %v", err)
	}
	if len(got) != 2 || got[0].ID == got[1].ID {
		t.Errorf("duplicating a layer = %+v, want two distinct layers", got)
	}
}

func TestWatermarkLayerRejectsOutOfRangeValues(t *testing.T) {
	s := openMemory(t)
	for name, mutate := range map[string]func(*store.WatermarkLayer){
		"size":    func(l *store.WatermarkLayer) { l.Size = 0 },
		"x":       func(l *store.WatermarkLayer) { l.X = 3 },
		"padding": func(l *store.WatermarkLayer) { l.Background.Padding[2] = -1 },
		"shadow":  func(l *store.WatermarkLayer) { l.Shadow.Color = "zzz" },
		"nan":     func(l *store.WatermarkLayer) { l.Y = math.NaN() },
	} {
		l := sampleLayer()
		mutate(&l)
		var invalid *config.InvalidValueError
		if err := s.UpsertWatermarkLayer(l); !errors.As(err, &invalid) {
			t.Errorf("%s: UpsertWatermarkLayer = %v, want InvalidValueError", name, err)
		}
	}
}

// TestOpenConvertsLegacyWatermarkLayers covers an upgrade: a database written
// before the editor holds point sizes and margins, which Open converts once.
func TestOpenConvertsLegacyWatermarkLayers(t *testing.T) {
	db, path := openAtVersion(t, 5)
	for _, stmt := range []string{
		`INSERT INTO settings (key, value) VALUES ('picture.reference-height', '1200')`,
		`INSERT INTO watermark_layers
		     (sort_order, font, point, color, position, h_margin, v_margin,
		      linespace, alignment, template, bg_color, bg_paddings)
		 VALUES (0, 'Arial.ttf', 24, 'ffffff', 'bottom-right', 0.05, 60,
		         1.2, 'right', '{{.Title}}', '00000080', '[0.5, 12]')`,
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

	layers, err := s.WatermarkLayers()
	if err != nil {
		t.Fatalf("WatermarkLayers: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(layers))
	}
	l := layers[0]
	near := func(name string, got, want float64) {
		if math.Abs(got-want) > 1e-3 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	// 24 points at a 1200-pixel reference height is 2 % of the shorter side.
	near("size", l.Size, 2)
	near("x", l.X, 0.95)
	// A 60-pixel margin at a 1200-pixel reference height.
	near("y", l.Y, 0.95)
	if l.Anchor != config.PositionBottomRight || l.Alignment != config.AlignRight {
		t.Errorf("anchor/alignment = %s/%s", l.Anchor, l.Alignment)
	}
	// [0.5, 12] is 0.5 of the line height vertically and 12 px (half the
	// 24-point font) horizontally.
	near("padding top", l.Background.Padding[0], 0.375)
	near("padding right", l.Background.Padding[1], 0.5)

	version, _ := s.SchemaVersion(MetaWatermarkLayers)
	if version != WatermarkLayersVersion {
		t.Errorf("watermark schema version = %d, want %d", version, WatermarkLayersVersion)
	}
}

func TestOpenRefusesNewerWatermarkSchema(t *testing.T) {
	path := t.TempDir() + "/gotapaper.db"
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSchemaVersion(MetaWatermarkLayers, WatermarkLayersVersion+1); err != nil {
		t.Fatal(err)
	}
	s.Close()

	if _, err := Open(path); !errors.Is(err, store.ErrSchemaTooNew) {
		t.Errorf("Open = %v, want ErrSchemaTooNew", err)
	}
}
