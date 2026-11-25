package store

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/genzj/goTApaper/config"
)

// MaxWatermarkLayers caps how many layers one configuration may hold. Each is
// rendered on every refresh and every preview; the cap keeps a runaway client
// from making either arbitrarily slow.
const MaxWatermarkLayers = 32

// NewWatermarkLayer returns a layer with every field at its default: small
// white text in the bottom-right corner, showing the picture's title.
func NewWatermarkLayer(font string) WatermarkLayer {
	return WatermarkLayer{
		SortOrder: AppendOrder,
		Enabled:   true,
		Template:  "{{.Title}}",
		Font:      font,
		Size:      1.6,
		Color:     "ffffff",
		Anchor:    config.PositionBottomRight,
		X:         0.97,
		Y:         0.95,
		Alignment: config.AlignRight,
		Linespace: 1.4,
		Shadow:    WatermarkShadow{Color: "00000088", DX: 0.06, DY: 0.06},
	}
}

// ValidateWatermarkLayer checks every field of a layer against its
// vocabulary and range, so a bad value is reported rather than rendered as a
// silent fallback. The template is not parsed here: its functions belong to
// the renderer, which the store cannot import.
func ValidateWatermarkLayer(l WatermarkLayer) error {
	if !config.ValidPosition(l.Anchor) {
		return &config.InvalidValueError{
			Key: "watermark.anchor", Value: l.Anchor,
			Kind: config.KindEnum, Expected: config.PositionValues,
		}
	}
	if !config.ValidAlignment(l.Alignment) {
		return &config.InvalidValueError{
			Key: "watermark.alignment", Value: l.Alignment,
			Kind: config.KindEnum, Expected: config.AlignmentValues,
		}
	}

	colors := []struct {
		key, value string
		optional   bool
	}{
		{"watermark.color", l.Color, false},
		{"watermark.background.color", l.Background.Color, true},
		{"watermark.shadow.color", l.Shadow.Color, true},
		{"watermark.outline.color", l.Outline.Color, true},
	}
	for _, c := range colors {
		if c.optional && c.value == "" {
			continue
		}
		if !ValidHexColor(c.value) {
			return &config.InvalidValueError{
				Key: c.key, Value: c.value, Kind: "colour (rgb, rrggbb or rrggbbaa)",
			}
		}
	}

	ranges := []struct {
		key      string
		value    float64
		min, max float64
	}{
		{"watermark.size", l.Size, 0.1, 50},
		{"watermark.x", l.X, -0.5, 1.5},
		{"watermark.y", l.Y, -0.5, 1.5},
		{"watermark.linespace", l.Linespace, 0.5, 10},
		{"watermark.background.padding", l.Background.Padding[0], 0, 20},
		{"watermark.background.padding", l.Background.Padding[1], 0, 20},
		{"watermark.background.padding", l.Background.Padding[2], 0, 20},
		{"watermark.background.padding", l.Background.Padding[3], 0, 20},
		{"watermark.shadow.dx", l.Shadow.DX, -5, 5},
		{"watermark.shadow.dy", l.Shadow.DY, -5, 5},
		{"watermark.outline.width", l.Outline.Width, 0, 2},
	}
	for _, r := range ranges {
		if math.IsNaN(r.value) || r.value < r.min || r.value > r.max {
			return &config.InvalidValueError{
				Key: r.key, Value: strconv.FormatFloat(r.value, 'g', -1, 64),
				Kind: config.Kind(fmt.Sprintf("number from %g to %g", r.min, r.max)),
			}
		}
	}

	if len(l.Template) > 4096 {
		return &config.InvalidValueError{
			Key: "watermark.template", Value: l.Template[:32] + "…",
			Kind: "template of at most 4096 bytes",
		}
	}
	if len(l.Name) > 200 {
		return &config.InvalidValueError{
			Key: "watermark.name", Value: l.Name[:32] + "…",
			Kind: "name of at most 200 bytes",
		}
	}
	return nil
}

// ValidHexColor reports whether s is a colour the renderer understands: three,
// six or eight hexadecimal digits, optionally prefixed with #.
func ValidHexColor(s string) bool {
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 3, 6, 8:
	default:
		return false
	}
	_, err := strconv.ParseUint(s, 16, 32)
	return err == nil
}

// LegacyWatermarkLayer is a layer in the shape stored before the editor:
// font size in points at the reference height, margins that were fractions
// below 1 and pixels from 1, and absolute-* switches.
type LegacyWatermarkLayer struct {
	Font             string
	Point            float64
	Color            string
	Position         string
	HMargin          float64
	VMargin          float64
	Linespace        float64
	Alignment        string
	Template         string
	AbsolutePoint    bool
	AbsolutePosition bool
	Background       struct {
		Color       string
		HThroughout bool
		VThroughout bool
		Paddings    []float64
	}
}

// legacyAssumedTextWidthEm is how wide the old renderer's text box is assumed
// to have been, in em, when converting a horizontal padding that was a
// fraction of it. The real width depended on the rendered text, which a
// conversion cannot know; this is a typical one-line caption.
const legacyAssumedTextWidthEm = 16

// legacyFontHeightEm is the old renderer's text-box height for one line, in
// em: gg reports a font's height as 72/96 of its size.
const legacyFontHeightEm = 0.75

// ConvertLegacyWatermark converts a layer of the old shape into the new
// picture-relative one, as it would have rendered on a picture of the
// reference size. refW and refH are picture.reference-width/height.
//
// Sizes and positions convert exactly for a landscape picture. Paddings given
// as fractions were relative to the text box, so they convert approximately;
// see legacyAssumedTextWidthEm.
func ConvertLegacyWatermark(old LegacyWatermarkLayer, refW, refH float64) WatermarkLayer {
	if refW <= 0 || math.IsNaN(refW) || math.IsInf(refW, 0) {
		refW = 1920
	}
	if refH <= 0 || math.IsNaN(refH) || math.IsInf(refH, 0) {
		refH = 1080
	}
	shorter := math.Min(refW, refH)

	// The old renderer scaled the point size by picture height over the
	// reference height, so on a reference-sized picture the font was point
	// pixels tall. An absolute point size was the same number of pixels,
	// which on a reference-sized picture is the same thing.
	point := old.Point
	if point <= 0 {
		point = 13
	}
	size := point / shorter * 100
	fontPx := point

	position := old.Position
	if !config.ValidPosition(position) {
		position = config.PositionBottomRight
	}
	alignment := old.Alignment
	if !config.ValidAlignment(alignment) {
		alignment = config.AlignRight
	}

	fraction := func(margin, extent float64) float64 {
		switch {
		case margin < 0:
			return 0.05
		case margin < 1:
			return margin
		default:
			return margin / extent
		}
	}
	hm := fraction(old.HMargin, refW)
	vm := fraction(old.VMargin, refH)

	x, y := 0.5, 0.5
	switch {
	case strings.HasSuffix(position, "-left"):
		x = hm
	case strings.HasSuffix(position, "-right"):
		x = 1 - hm
	}
	switch {
	case strings.HasPrefix(position, "top-"):
		y = vm
	case strings.HasPrefix(position, "bottom-"):
		y = 1 - vm
	}

	linespace := old.Linespace
	if linespace <= 0 {
		linespace = 1.4
	}

	return WatermarkLayer{
		SortOrder: AppendOrder,
		Enabled:   true,
		Template:  old.Template,
		Font:      old.Font,
		Size:      round4(size),
		Color:     orHex(old.Color, "222222"),
		Anchor:    position,
		X:         round4(x),
		Y:         round4(y),
		Alignment: alignment,
		Linespace: linespace,
		Background: WatermarkBackground{
			Color:       old.Background.Color,
			HThroughout: old.Background.HThroughout,
			VThroughout: old.Background.VThroughout,
			Padding:     convertLegacyPaddings(old.Background.Paddings, fontPx),
		},
	}
}

// convertLegacyPaddings expands the old one-to-four value padding list into
// top, right, bottom, left, and converts each value to em.
func convertLegacyPaddings(p []float64, fontPx float64) [4]float64 {
	var trbl [4]float64
	switch len(p) {
	case 0:
	case 1:
		trbl = [4]float64{p[0], p[0], p[0], p[0]}
	case 2:
		trbl = [4]float64{p[0], p[1], p[0], p[1]}
	case 3:
		trbl = [4]float64{p[0], p[1], p[2], p[1]}
	default:
		trbl = [4]float64{p[0], p[1], p[2], p[3]}
	}

	for i, v := range trbl {
		vertical := i == 0 || i == 2
		switch {
		case math.Abs(v) < 1 && vertical:
			v *= legacyFontHeightEm
		case math.Abs(v) < 1:
			v *= legacyAssumedTextWidthEm
		default:
			v /= fontPx
		}
		trbl[i] = round4(math.Min(math.Max(v, 0), 20))
	}
	return trbl
}

func orHex(s, fallback string) string {
	if ValidHexColor(s) {
		return s
	}
	return fallback
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
