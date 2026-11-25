package watermark

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// TestMain points the font index at a fixture directory holding the Go fonts,
// so the tests neither depend on nor scan the machine's fonts.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gotapaper-fonts-")
	if err != nil {
		panic(err)
	}
	for name, data := range map[string][]byte{
		"Go-Regular.ttf": goregular.TTF,
		"Go-Bold.ttf":    gobold.TTF,
		// Not a font at all: must be left out of the index.
		"Broken.ttf": []byte("definitely not a font"),
		// A real font the renderer's parser rejects, as it does Apple's
		// version 1 kern tables: reported as unsupported, not offered.
		"Go-Unsupported.ttf": unsupportedFont(),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			panic(err)
		}
	}
	fontDirectories = func() []string { return []string{dir} }
	realParse := parseFont
	parseFont = func(data []byte) (*truetype.Font, error) {
		if bytes.HasSuffix(data, []byte(unsupportedMarker)) {
			return nil, truetype.UnsupportedError("kern version: 1")
		}
		return realParse(data)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// unsupportedMarker tags a fixture font the stand-in parser rejects.
const unsupportedMarker = "\x00gotapaper-unsupported"

// unsupportedFont is the Go font with a marker appended, which both parsers
// ignore. TestMain's stand-in for the renderer's parser rejects it the way
// golang/freetype rejects a font with Apple's version 1 kern table, so the
// test needs no such font.
func unsupportedFont() []byte {
	return append(append([]byte(nil), goregular.TTF...), unsupportedMarker...)
}

func solid(w, h int, c color.Color) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(im, im.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return im
}

func testLayer() store.WatermarkLayer {
	l := store.NewWatermarkLayer("Go-Regular.ttf")
	l.Template = "{{.Title}}"
	l.Size = 6
	l.Shadow = store.WatermarkShadow{}
	return l
}

var testMeta = &channel.PictureMeta{Title: "Lake Bled at dawn", Credit: "Jane Doe"}

func TestFontIndexListsLoadableFontsOnly(t *testing.T) {
	all := Fonts()
	if len(all) != 2 {
		t.Fatalf("indexed %d fonts, want the two Go fonts: %+v", len(all), all)
	}
	if all[0].Family != "Go" || all[0].Style != "Regular" {
		t.Errorf("first font = %s %s, want Go Regular first", all[0].Family, all[0].Style)
	}
	if f, ok := FontByID(all[1].ID); !ok || f.File != "Go-Bold.ttf" {
		t.Errorf("FontByID = %+v, %v", f, ok)
	}

	for name, want := range map[string]string{
		"Go-Bold.ttf": "Go-Bold.ttf",
		"go-bold":     "Go-Bold.ttf",
		"Regular":     "Go-Regular.ttf",
	} {
		path, err := findFont(name)
		if err != nil || filepath.Base(path) != want {
			t.Errorf("findFont(%q) = %q, %v; want %s", name, path, err, want)
		}
	}
	if _, err := findFont("NoSuchFont.ttf"); err == nil {
		t.Error("findFont found a font that does not exist")
	}
	bad := UnsupportedFonts()
	if len(bad) != 1 || bad[0].File != "Go-Unsupported.ttf" || bad[0].Reason == "" {
		t.Fatalf("UnsupportedFonts = %+v, want the relabelled font with a reason", bad)
	}
	if !strings.Contains(bad[0].Reason, "kern version: 1") {
		t.Errorf("reason = %q, want it to name the feature", bad[0].Reason)
	}
	if _, err := findFont("Go-Unsupported.ttf"); err == nil || !strings.Contains(err.Error(), "pick another font") {
		t.Errorf("findFont(unsupported) = %v, want an error saying to pick another font", err)
	}
	if got := DefaultFont(); got != "Go-Regular.ttf" {
		t.Errorf("DefaultFont = %q, want the only regular face", got)
	}
}

// TestRenderIsResolutionIndependent is the property the picture-relative units
// exist for: the same layer lands in the same place, at the same relative size,
// on a small preview and on a 4K picture.
func TestRenderIsResolutionIndependent(t *testing.T) {
	layers := []store.WatermarkLayer{testLayer()}
	layers[0].Background = store.WatermarkBackground{Color: "00000080", Padding: [4]float64{0.3, 0.5, 0.3, 0.5}}

	_, small := RenderLayers(solid(960, 540, color.White), testMeta, layers, RenderOptions{})
	_, large := RenderLayers(solid(3840, 2160, color.White), testMeta, layers, RenderOptions{})

	for _, r := range [][]LayerResult{small, large} {
		if r[0].Error != nil {
			t.Fatalf("render error: %v", r[0].Error)
		}
	}
	a, b := small[0].TextBounds, large[0].TextBounds
	for name, d := range map[string]float64{
		"x": a.X - b.X, "y": a.Y - b.Y, "w": a.W - b.W, "h": a.H - b.H,
		"box w": small[0].BoxBounds.W - large[0].BoxBounds.W,
	} {
		if math.Abs(d) > 0.005 {
			t.Errorf("%s differs by %.4f of the picture between 960 and 3840 pixels (%+v vs %+v)", name, d, a, b)
		}
	}
	// Anchored bottom-right at 0.97, 0.95.
	if right := b.X + b.W; math.Abs(right-0.97) > 1e-6 {
		t.Errorf("text right edge = %v, want 0.97", right)
	}
	if bottom := b.Y + b.H; math.Abs(bottom-0.95) > 1e-6 {
		t.Errorf("text bottom edge = %v, want 0.95", bottom)
	}
	if want := 0.06 * 2160; math.Abs(large[0].FontPx-want) > 1e-9 {
		t.Errorf("FontPx = %v, want %v", large[0].FontPx, want)
	}
}

func TestRenderDrawsInsideTheReportedBounds(t *testing.T) {
	l := testLayer()
	l.Color = "ff0000"
	out, res := RenderLayers(solid(800, 600, color.Black), testMeta, []store.WatermarkLayer{l}, RenderOptions{})
	tb := res[0].TextBounds

	inside, outside := 0, 0
	b := out.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := out.At(x, y).RGBA()
			if r == 0 {
				continue
			}
			fx, fy := (float64(x)+0.5)/800, (float64(y)+0.5)/600
			// A pixel of slack for antialiasing and glyph overhang.
			if fx >= tb.X-2.0/800 && fx <= tb.X+tb.W+2.0/800 && fy >= tb.Y-4.0/600 && fy <= tb.Y+tb.H+4.0/600 {
				inside++
			} else {
				outside++
			}
		}
	}
	if inside == 0 {
		t.Fatal("nothing was drawn")
	}
	if outside > inside/50 {
		t.Errorf("%d pixels drawn outside the reported bounds (%d inside)", outside, inside)
	}
}

// TestRenderHandlesASubImage covers the cropper's output, whose bounds do not
// start at the origin.
func TestRenderHandlesASubImage(t *testing.T) {
	big := solid(1000, 1000, color.White)
	sub := big.SubImage(image.Rect(200, 300, 1000, 900))

	l := testLayer()
	l.Background = store.WatermarkBackground{Color: "000000", HThroughout: true, VThroughout: true}
	out, res := RenderLayers(sub, testMeta, []store.WatermarkLayer{l}, RenderOptions{})
	if res[0].Error != nil {
		t.Fatal(res[0].Error)
	}
	if b := out.Bounds(); b.Dx() != 800 || b.Dy() != 600 || b.Min != (image.Point{}) {
		t.Fatalf("output bounds = %v, want 800x600 at the origin", b)
	}
	// A full-picture background covers the corners.
	for _, p := range []image.Point{{0, 0}, {799, 599}} {
		if r, _, _, _ := out.At(p.X, p.Y).RGBA(); r != 0 {
			t.Errorf("pixel %v is not covered by the background", p)
		}
	}
}

func TestRenderReturnsTheSourceWhenNothingIsEnabled(t *testing.T) {
	src := solid(100, 100, color.White)
	l := testLayer()
	l.Enabled = false
	out, res := RenderLayers(src, testMeta, []store.WatermarkLayer{l}, RenderOptions{})
	if out != image.Image(src) {
		t.Error("an image with no enabled layer was copied; its original bytes would be re-encoded")
	}
	if res[0].Enabled {
		t.Error("disabled layer reported as enabled")
	}
}

func TestRenderReportsLayerErrors(t *testing.T) {
	badTemplate := testLayer()
	badTemplate.Template = "{{.Title}}\n{{.NoSuchField}}"
	badSyntax := testLayer()
	badSyntax.Template = "{{.Title"
	badFont := testLayer()
	badFont.Font = "NoSuchFont.ttf"
	fine := testLayer()

	_, res := RenderLayers(solid(400, 300, color.White), testMeta,
		[]store.WatermarkLayer{badTemplate, badSyntax, badFont, fine}, RenderOptions{})

	if e := res[0].Error; e == nil || e.Stage != "template" || e.Line != 2 || e.Column == 0 {
		t.Errorf("unknown field error = %+v, want a template error at line 2 with a column", e)
	}
	if e := res[1].Error; e == nil || e.Stage != "template" || e.Line != 1 {
		t.Errorf("syntax error = %+v, want a template error at line 1", e)
	}
	if e := res[2].Error; e == nil || e.Stage != "font" {
		t.Errorf("missing font = %+v, want a font error", e)
	}
	if res[3].Error != nil || res[3].Text != "Lake Bled at dawn" {
		t.Errorf("valid layer = %+v", res[3])
	}
}

func TestRenderEffectsAndDebugOverlay(t *testing.T) {
	l := testLayer()
	l.Shadow = store.WatermarkShadow{Color: "000000", DX: 0.1, DY: 0.1}
	l.Outline = store.WatermarkOutline{Color: "0000ff", Width: 0.05}

	plain, _ := RenderLayers(solid(600, 400, color.White), testMeta, []store.WatermarkLayer{testLayer()}, RenderOptions{})
	styled, _ := RenderLayers(solid(600, 400, color.White), testMeta, []store.WatermarkLayer{l}, RenderOptions{})
	debug, _ := RenderLayers(solid(600, 400, color.White), testMeta, []store.WatermarkLayer{l}, RenderOptions{Debug: true})

	if changed(plain, styled) == 0 {
		t.Error("shadow and outline drew nothing")
	}
	if changed(styled, debug) == 0 {
		t.Error("debug overlay drew nothing")
	}
}

func changed(a, b image.Image) int {
	n := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.At(x, y) != b.At(x, y) {
				n++
			}
		}
	}
	return n
}

// TestTemplateFieldsCoverPictureMeta keeps the editor's help from drifting: a
// field added to PictureMeta must be documented here.
func TestTemplateFieldsCoverPictureMeta(t *testing.T) {
	documented := map[string]bool{}
	for _, f := range TemplateFields {
		documented[f.Name] = true
		if f.Description == "" || f.Example == "" || f.Snippet == "" {
			t.Errorf("field %s is missing help", f.Name)
		}
		if err := CheckTemplate(f.Snippet); err != nil {
			t.Errorf("snippet of %s does not parse: %v", f.Name, err)
		}
	}
	mt := reflect.TypeOf(channel.PictureMeta{})
	for i := 0; i < mt.NumField(); i++ {
		if f := mt.Field(i); f.IsExported() && !documented[f.Name] {
			t.Errorf("PictureMeta.%s is not in TemplateFields", f.Name)
		}
	}
}

func TestTemplateFuncExamples(t *testing.T) {
	want := map[string]string{
		"date":     "Nov 24, 2026",
		"upper":    "LAKE BLED AT DAWN",
		"lower":    "bing-wallpaper",
		"default":  "Untitled",
		"truncate": "Lake Bled…",
		"join":     "Lake Bled at dawn · © Jane Doe / Example Images",
	}
	if len(want) != len(TemplateFuncs) {
		t.Fatalf("%d functions documented, %d expected here", len(TemplateFuncs), len(want))
	}
	for _, f := range TemplateFuncs {
		if f.Result != want[f.Name] {
			t.Errorf("%s: %s renders %q, want %q", f.Name, f.Example, f.Result, want[f.Name])
		}
		if err := CheckTemplate(f.Snippet); err != nil {
			t.Errorf("snippet of %s does not parse: %v", f.Name, err)
		}
	}
}

func TestFieldsForFillsCurrentValues(t *testing.T) {
	fields := FieldsFor(&exampleMeta)
	for _, f := range fields {
		if f.Name == "UploadTime" && f.Current != "2026-11-24" {
			t.Errorf("UploadTime current = %q", f.Current)
		}
		if f.Name == "Title" && f.Current != "Lake Bled at dawn" {
			t.Errorf("Title current = %q", f.Current)
		}
	}
	if FieldsFor(nil)[0].Current != "" {
		t.Error("a field has a current value without a picture")
	}
}

func TestPresetsAreValidAndRender(t *testing.T) {
	presets := Presets()
	if len(presets) < 5 {
		t.Fatalf("only %d presets", len(presets))
	}
	seen := map[string]bool{}
	for _, p := range presets {
		if seen[p.ID] {
			t.Errorf("duplicate preset id %s", p.ID)
		}
		seen[p.ID] = true

		for i, l := range p.Layers {
			if err := store.ValidateWatermarkLayer(l); err != nil {
				t.Errorf("%s layer %d: %v", p.ID, i, err)
			}
			if !config.ValidPosition(l.Anchor) {
				t.Errorf("%s layer %d anchor %q", p.ID, i, l.Anchor)
			}
		}
		_, res := RenderLayers(solid(1920, 1080, color.Gray{Y: 128}), &exampleMeta, p.Layers, RenderOptions{})
		for i, r := range res {
			if r.Error != nil {
				t.Errorf("%s layer %d: %v", p.ID, i, r.Error)
			}
			tb := r.TextBounds
			if r.Text != "" && (tb.X < 0 || tb.Y < 0 || tb.X+tb.W > 1 || tb.Y+tb.H > 1) {
				t.Errorf("%s layer %d runs off the picture: %+v", p.ID, i, tb)
			}
		}
	}
}
