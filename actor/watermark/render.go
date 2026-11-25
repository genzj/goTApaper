package watermark

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fogleman/gg"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// anchorFactors maps an anchor to the fraction of the text box's width and
// height that lies before the anchor point.
var anchorFactors = map[string][2]float64{
	config.PositionTopLeft:      {0, 0},
	config.PositionTopCenter:    {0.5, 0},
	config.PositionTopRight:     {1, 0},
	config.PositionMiddleLeft:   {0, 0.5},
	config.PositionMiddleCenter: {0.5, 0.5},
	config.PositionMiddleRight:  {1, 0.5},
	config.PositionBottomLeft:   {0, 1},
	config.PositionBottomCenter: {0.5, 1},
	config.PositionBottomRight:  {1, 1},
}

var alignments = map[string]gg.Align{
	config.AlignLeft:   gg.AlignLeft,
	config.AlignCenter: gg.AlignCenter,
	config.AlignRight:  gg.AlignRight,
}

// fallbackFontColor is used when a colour fails to parse. Stored layers are
// validated, so this only guards a draft sent straight to a preview.
const fallbackFontColor = "222222"

type render struct {
	ctx *gg.Context
}

// rect is an area in pixels.
type rect struct{ x, y, w, h float64 }

// layout is where one layer's text and background box go, in pixels.
type layout struct {
	text, box rect
	width     float64 // the widest line, which is the text block's width
}

func (r *render) size() (w, h float64) {
	return float64(r.ctx.Width()), float64(r.ctx.Height())
}

// fontPx converts a layer's size, a percentage of the picture's shorter side,
// into pixels. Not rounded: rounding would make a preview and the full-size
// picture disagree by up to half a pixel of font size.
func (r *render) fontPx(l store.WatermarkLayer) float64 {
	w, h := r.size()
	return math.Max(l.Size/100*math.Min(w, h), 1)
}

// loadFont makes the layer's font current and returns its path.
func (r *render) loadFont(l store.WatermarkLayer) (string, error) {
	path, err := findFont(l.Font)
	if err != nil {
		return "", err
	}
	if err := r.ctx.LoadFontFace(path, r.fontPx(l)); err != nil {
		// A path given directly skips the index's check, so the parser's
		// error can still surface here.
		return "", fmt.Errorf("font %s %s; pick another font", filepath.Base(path), describeFontError(err))
	}
	return path, nil
}

// layout measures a layer's text and places it. The current font must be the
// layer's.
func (r *render) layout(l store.WatermarkLayer, text string) layout {
	w, h := r.size()
	em := r.fontPx(l)

	width := 0.0
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if lw, _ := r.ctx.MeasureString(line); lw > width {
			width = lw
		}
	}
	// Kept in step with gg's DrawStringWrapped, which is what draws it.
	fh := r.ctx.FontHeight()
	height := float64(len(lines))*fh*l.Linespace - (l.Linespace-1)*fh

	f, ok := anchorFactors[l.Anchor]
	if !ok {
		f = anchorFactors[config.PositionBottomRight]
	}
	text0 := rect{
		x: l.X*w - f[0]*width,
		y: l.Y*h - f[1]*height,
		w: width,
		h: height,
	}

	p := l.Background.Padding
	x0, y0 := text0.x-p[3]*em, text0.y-p[0]*em
	x1, y1 := text0.x+text0.w+p[1]*em, text0.y+text0.h+p[2]*em
	if l.Background.HThroughout {
		x0, x1 = 0, w
	}
	if l.Background.VThroughout {
		y0, y1 = 0, h
	}
	x0, y0 = math.Max(x0, 0), math.Max(y0, 0)
	x1, y1 = math.Min(x1, w), math.Min(y1, h)

	return layout{
		text:  text0,
		box:   rect{x: x0, y: y0, w: math.Max(x1-x0, 0), h: math.Max(y1-y0, 0)},
		width: width,
	}
}

// relative converts a pixel area into fractions of the picture.
func (r *render) relative(p rect) Rect {
	w, h := r.size()
	return Rect{X: p.x / w, Y: p.y / h, W: p.w / w, H: p.h / h}
}

func (r *render) drawBackground(l store.WatermarkLayer, lay layout) {
	if l.Background.Color == "" || lay.box.w <= 0 || lay.box.h <= 0 {
		return
	}
	r.ctx.DrawRectangle(lay.box.x, lay.box.y, lay.box.w, lay.box.h)
	r.ctx.SetHexColor(l.Background.Color)
	r.ctx.Fill()
}

// drawText draws the shadow, then the outline, then the text itself.
//
// gg has no stroked text, so the outline is the text drawn at evenly spaced
// offsets around a circle of the outline width -- enough of them that no gap
// shows at that radius.
func (r *render) drawText(l store.WatermarkLayer, text string, lay layout) {
	em := r.fontPx(l)
	align, ok := alignments[l.Alignment]
	if !ok {
		align = gg.AlignRight
	}
	draw := func(dx, dy float64) {
		r.ctx.DrawStringWrapped(text, lay.text.x+dx, lay.text.y+dy, 0, 0, lay.width, l.Linespace, align)
	}

	if l.Shadow.Color != "" {
		r.ctx.SetHexColor(l.Shadow.Color)
		draw(l.Shadow.DX*em, l.Shadow.DY*em)
	}

	if radius := l.Outline.Width * em; l.Outline.Color != "" && radius > 0 {
		r.ctx.SetHexColor(l.Outline.Color)
		steps := int(math.Min(math.Max(math.Ceil(2*math.Pi*radius/1.5), 8), 48))
		for i := 0; i < steps; i++ {
			a := 2 * math.Pi * float64(i) / float64(steps)
			draw(radius*math.Cos(a), radius*math.Sin(a))
		}
	}

	r.ctx.SetHexColor(fallbackFontColor)
	if store.ValidHexColor(l.Color) {
		r.ctx.SetHexColor(l.Color)
	}
	draw(0, 0)
}

// drawDebugOverlay marks the picture's bounds and centre lines and, for each
// drawn layer, its background box, its text block and its anchor point, with
// a label naming the layer and anchor.
func (r *render) drawDebugOverlay(layers []store.WatermarkLayer, results []LayerResult, fontPaths []string) {
	w, h := r.size()
	line := math.Max(1, math.Min(w, h)/400)
	ctx := r.ctx

	ctx.Push()
	defer ctx.Pop()

	// Picture bounds and centre lines.
	ctx.SetLineWidth(line * 2)
	ctx.SetHexColor("ffff00cc")
	ctx.DrawRectangle(line, line, w-2*line, h-2*line)
	ctx.Stroke()
	ctx.SetLineWidth(line)
	ctx.SetDash(line*8, line*6)
	ctx.DrawLine(0, h/2, w, h/2)
	ctx.DrawLine(w/2, 0, w/2, h)
	ctx.Stroke()
	ctx.SetDash()

	for i, res := range results {
		if !res.Enabled || res.Error != nil || res.Text == "" {
			continue
		}
		l := layers[i]
		box, text := res.BoxBounds, res.TextBounds

		// Background box, padding included.
		ctx.SetHexColor("ff00ffcc")
		ctx.SetLineWidth(line)
		ctx.DrawRectangle(box.X*w, box.Y*h, box.W*w, box.H*h)
		ctx.Stroke()

		// Text block.
		ctx.SetHexColor("ff3030ee")
		ctx.SetLineWidth(line * 1.5)
		ctx.DrawRectangle(text.X*w, text.Y*h, text.W*w, text.H*h)
		ctx.Stroke()

		// Anchor point.
		ax, ay := l.X*w, l.Y*h
		arm := line * 10
		ctx.SetHexColor("00e5ffee")
		ctx.SetLineWidth(line * 2)
		ctx.DrawCircle(ax, ay, arm/2)
		ctx.DrawLine(ax-arm, ay, ax+arm, ay)
		ctx.DrawLine(ax, ay-arm, ax, ay+arm)
		ctx.Stroke()

		// Label, in the layer's own font at a legible size.
		if fontPaths[i] == "" {
			continue
		}
		if err := ctx.LoadFontFace(fontPaths[i], math.Max(12, math.Min(w, h)/70)); err != nil {
			continue
		}
		name := l.Name
		if name == "" {
			name = "Layer " + strconv.Itoa(i+1)
		}
		label := name + " · " + l.Anchor
		lw, lh := ctx.MeasureString(label)
		lx := math.Min(math.Max(text.X*w, 0), w-lw-line*4)
		ly := text.Y*h - lh - line*4
		if ly < 0 {
			ly = (text.Y+text.H)*h + line*2
		}
		ctx.SetHexColor("000000aa")
		ctx.DrawRectangle(lx, ly, lw+line*4, lh+line*3)
		ctx.Fill()
		ctx.SetHexColor("00e5ff")
		ctx.DrawStringAnchored(label, lx+line*2, ly+line, 0, 1)
	}
}
