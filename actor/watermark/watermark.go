// Package watermark draws text overlays onto a wallpaper.
//
// Every size and position in a layer is relative to the picture -- see
// store.WatermarkLayer -- so a layer renders identically, up to rounding, on a
// small preview and on the full-size picture. That is what lets the editor
// preview at any resolution without a scale parameter.
package watermark

import (
	"image"
	"image/draw"
	"strings"

	"github.com/fogleman/gg"
	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/util"
)

// RenderOptions adjusts one render.
type RenderOptions struct {
	// Debug draws each layer's bounds, background box and anchor, with the
	// picture's centre lines, over the result.
	Debug bool
}

// Rect is an area of the picture, as fractions of its width and height.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// LayerError explains why a layer was not drawn.
type LayerError struct {
	// Stage is "template" or "font".
	Stage   string `json:"stage"`
	Message string `json:"message"`
	// Line and Column locate a template error, one-based, zero if unknown.
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

func (e *LayerError) Error() string { return e.Stage + ": " + e.Message }

// LayerResult reports how one layer rendered, in the order the layers were
// given, so an editor can draw a handle over each and show its problems.
type LayerResult struct {
	// Index is the layer's position in the list rendered. A draft layer that
	// has never been saved has no ID, so this is what identifies it.
	Index   int   `json:"index"`
	ID      int64 `json:"id"`
	Enabled bool  `json:"enabled"`
	// Text is the template's output.
	Text string `json:"text"`
	// TextBounds is the text block and BoxBounds the background box around
	// it, including padding.
	TextBounds Rect `json:"textBounds"`
	BoxBounds  Rect `json:"boxBounds"`
	// FontPx is the font size on this picture, in pixels, for the editor's
	// "≈ N px" readout.
	FontPx float64     `json:"fontPx"`
	Error  *LayerError `json:"error,omitempty"`
}

// Render draws the enabled layers onto an image for a refresh. Rendering
// problems are logged and the layer skipped: the wallpaper is still worth
// applying without it.
//
// When the debug-rendering setting is on, the debug overlay is drawn and the
// picture as it was before rendering is saved next to the wallpaper file.
func Render(im image.Image, meta *channel.PictureMeta, layers []store.WatermarkLayer) (image.Image, error) {
	debug := config.GetBool(config.KeyLogDebugRendering)
	if debug {
		path := config.GetWallpaperFileName() + "-before-rendering.jpeg"
		if err := util.SaveImageToJpeg(im, path, 75); err != nil {
			logrus.WithError(err).Debug("cannot save the picture before rendering")
		}
	}

	out, results := RenderLayers(im, meta, layers, RenderOptions{Debug: debug})
	for _, r := range results {
		if r.Error != nil {
			logrus.WithFields(logrus.Fields{
				"layer": r.Index + 1,
				"stage": r.Error.Stage,
			}).WithError(r.Error).Warn("watermark layer skipped")
		}
	}
	return out, nil
}

// RenderLayers draws the layers onto a copy of im and reports how each one
// rendered. Disabled layers are reported but not drawn. The source image is
// returned unchanged when there is nothing to draw, so a caller can keep the
// original file's bytes.
//
// Layers are drawn in order, each with its background then its text, so a
// later layer is on top of an earlier one -- what a layer list in an editor
// means.
func RenderLayers(
	im image.Image, meta *channel.PictureMeta, layers []store.WatermarkLayer, opts RenderOptions,
) (image.Image, []LayerResult) {
	results := make([]LayerResult, len(layers))
	anyEnabled := false
	for i, l := range layers {
		results[i] = LayerResult{Index: i, ID: l.ID, Enabled: l.Enabled}
		anyEnabled = anyEnabled || l.Enabled
	}
	if !anyEnabled && !opts.Debug {
		return im, results
	}
	if meta == nil {
		meta = &channel.PictureMeta{}
	}

	r := newRender(im)
	fontPaths := make([]string, len(layers))
	for i, l := range layers {
		if !l.Enabled {
			continue
		}
		res := &results[i]

		text, terr := executeTemplate(l.Template, meta)
		if terr != nil {
			res.Error = terr
			continue
		}
		res.Text = text

		path, err := r.loadFont(l)
		if err != nil {
			res.Error = &LayerError{Stage: "font", Message: err.Error()}
			continue
		}
		fontPaths[i] = path
		res.FontPx = r.fontPx(l)

		if text == "" {
			continue
		}
		lay := r.layout(l, text)
		res.TextBounds = r.relative(lay.text)
		res.BoxBounds = r.relative(lay.box)

		r.drawBackground(l, lay)
		r.drawText(l, text, lay)
	}

	if opts.Debug {
		r.drawDebugOverlay(layers, results, fontPaths)
	}
	return r.ctx.Image(), results
}

// executeTemplate renders a layer's template against the picture metadata.
func executeTemplate(src string, meta *channel.PictureMeta) (string, *LayerError) {
	tmpl, err := ParseTemplate(src)
	if err != nil {
		return "", templateError(err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, meta); err != nil {
		return "", templateError(err)
	}
	return strings.TrimSpace(b.String()), nil
}

// newRender prepares a context over a copy of im whose bounds start at the
// origin.
//
// The cropper returns a sub-image, whose bounds do not start at zero. gg
// draws text in the image's own coordinates but fills shapes from zero, so
// the two would disagree; normalising once here means every coordinate below
// is simply 0..W, 0..H.
func newRender(im image.Image) *render {
	b := im.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), im, b.Min, draw.Src)
	return &render{ctx: gg.NewContextForRGBA(rgba)}
}
