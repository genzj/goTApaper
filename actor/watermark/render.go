package watermark

import (
	"image"
	"math"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/fogleman/gg"
)

const (
	positionTopLeft      = "top-left"
	positionTopCenter    = "top-center"
	positionTopRight     = "top-right"
	positionMiddleLeft   = "middle-left"
	positionMiddleCenter = "middle-center"
	positionMiddleRight  = "middle-right"
	positionBottomLeft   = "bottom-left"
	positionBottomCenter = "bottom-center"
	positionBottomRight  = "bottom-right"
)

const (
	alignLeft   = "left"
	alignCenter = "center"
	alignRight  = "right"
)

const (
	fallbackFontColor       = "222222"
	fallbackBackgroundColor = "eeeeee77"
	fallbackPosition        = positionBottomRight
	fallbackAlign           = alignRight
)

type render struct {
	ctx     *gg.Context
	setting watermarkSetting
}

func newRender(im image.Image, setting watermarkSetting) render {
	return render{
		gg.NewContextForImage(im),
		setting,
	}
}

func (r render) loadBackgroundColor() {
	// in case the color in configuration not work
	r.ctx.SetHexColor(fallbackBackgroundColor)
	r.ctx.SetHexColor(r.setting.Background.Color)
}

func (r render) loadTextColor() {
	// in case the color in configuration not work
	r.ctx.SetHexColor(fallbackFontColor)
	r.ctx.SetHexColor(r.setting.Color)
}

func (r render) loadFont() error {
	pixelDense := float64(1.0)
	if r.setting.ReferenceHeight > 0 {
		pixelDense = float64(r.ctx.Height()) / 1080.0
	}
	fontFile, err := findFont(r.setting.Font)
	if err != nil {
		logrus.WithError(err).Errorf("cannot find font %s", r.setting.Font)
		return err
	}
	fontPoints := math.Round(float64(r.setting.Point) * pixelDense)
	logrus.WithFields(map[string]interface{}{
		"setFontPoints": r.setting.Point,
		"pixelDense":    pixelDense,
		"relFontPoints": fontPoints,
	}).Debug("load font face")
	err = r.ctx.LoadFontFace(fontFile, fontPoints)
	if err != nil {
		logrus.WithError(err).Errorf("cannot load font %s", fontFile)
		return err
	}

	return nil
}

func (r render) position(text string) (x, y, ax, ay, width float64) {
	var maxLineWidth float64 = 0
	vMargin := r.setting.VMargin
	hMargin := r.setting.HMargin

	filledHeight := float64(r.ctx.Height())
	filledWidth := float64(r.ctx.Width())
	var vCut, hCut float64
	if r.setting.ReferenceHeight > 0 && r.setting.ReferenceWidth > 0 {
		filledWidth, filledHeight = r.sizeAfterFill()
		hCut, vCut = r.cutAfterFill()
		logrus.WithField(
			"h", filledHeight,
		).WithField(
			"w", filledWidth,
		).Debug("filled size")
	}

	if vMargin < 0 {
		vMargin = 0.05
	}
	if hMargin < 0 {
		hMargin = 0.05
	}

	if vMargin < 1 {
		vMargin *= filledHeight
	}

	if hMargin < 1 {
		hMargin *= filledWidth
	}

	topOffset := func() float64 {
		return vCut + vMargin
	}
	bottomOffset := func() float64 {
		return float64(r.ctx.Height()) - vCut - vMargin
	}
	leftOffset := func() float64 {
		return hCut + hMargin
	}
	rightOffset := func() float64 {
		return float64(r.ctx.Width()) - hCut - hMargin
	}
	halfWidth := func() float64 {
		return float64(r.ctx.Width()) / 2
	}
	halfHeight := func() float64 {
		return float64(r.ctx.Height()) / 2
	}

	xCalculator := map[string]func() float64{
		positionTopLeft:      leftOffset,
		positionTopCenter:    halfWidth,
		positionTopRight:     rightOffset,
		positionMiddleLeft:   leftOffset,
		positionMiddleCenter: halfWidth,
		positionMiddleRight:  rightOffset,
		positionBottomLeft:   leftOffset,
		positionBottomCenter: halfWidth,
		positionBottomRight:  rightOffset,
	}
	yCalculator := map[string]func() float64{
		positionTopLeft:      topOffset,
		positionTopCenter:    topOffset,
		positionTopRight:     topOffset,
		positionMiddleLeft:   halfHeight,
		positionMiddleCenter: halfHeight,
		positionMiddleRight:  halfHeight,
		positionBottomLeft:   bottomOffset,
		positionBottomCenter: bottomOffset,
		positionBottomRight:  bottomOffset,
	}
	aCalculator := map[string][2]float64{
		positionTopLeft:      [2]float64{0, 0},
		positionTopCenter:    [2]float64{0.5, 0},
		positionTopRight:     [2]float64{1, 0},
		positionMiddleLeft:   [2]float64{0, 0.5},
		positionMiddleCenter: [2]float64{0.5, 0.5},
		positionMiddleRight:  [2]float64{1, 0.5},
		positionBottomLeft:   [2]float64{0, 1},
		positionBottomCenter: [2]float64{0.5, 1},
		positionBottomRight:  [2]float64{1, 1},
	}

	position := r.setting.Position
	if _, ok := xCalculator[position]; !ok {
		logrus.Warnf("invalid position %s, fallback to %s", position, fallbackPosition)
		position = fallbackPosition
	}

	x = xCalculator[position]()
	y = yCalculator[position]()
	a := aCalculator[position]
	ax, ay = a[0], a[1]
	logrus.Debugf("watermark position: %##v", map[string]float64{
		"x": x, "y": y,
		"ax": ax, "ay": ay,
	})

	for _, line := range strings.Split(text, "\n") {
		if lineW, _ := r.ctx.MeasureString(line); lineW > maxLineWidth {
			maxLineWidth = lineW
		}
	}

	return x, y, ax, ay, maxLineWidth
}

func (r render) boundOf(s string, x, y, ax, ay, width float64) (bx, by, bw, bh float64) {
	lineSpacing := r.setting.Linespace
	dc := r.ctx
	lines := dc.WordWrap(s, width)

	logrus.WithFields(map[string]interface{}{
		"fontheight":  dc.FontHeight(),
		"width":       width,
		"lineSpacing": lineSpacing,
		"lines":       lines,
	}).Debug("calculating bound")

	// sync h formula with MeasureMultilineString
	height := float64(len(lines)) * dc.FontHeight() * lineSpacing
	logrus.Debugf("height 1 %v", height)
	height -= (lineSpacing - 1) * dc.FontHeight()
	logrus.Debugf("height 2 %v", height)

	x -= ax * width
	y -= ay * height
	logrus.WithField("ax", ax).WithField("ay", ay).WithField("width", width).WithField("height", height).Debug("calculating bound")

	paddingSettings := r.setting.Background.Paddings
	paddings := make([]float64, 0, 4)
	switch l := len(paddingSettings); l {
	case 0:
		paddings = []float64{0, 0, 0, 0}
	case 1:
		paddings = append(paddings, paddingSettings[0], paddingSettings[0], paddingSettings[0], paddingSettings[0])
	case 2:
		paddings = append(paddings, paddingSettings[:2]...)
		paddings = append(paddings, paddingSettings[:2]...)
	case 3:
		paddings = append(paddings, paddingSettings[:3]...)
		paddings = append(paddings, paddingSettings[1])
	default:
		paddings = append(paddings, paddingSettings[:4]...)
	}
	logrus.WithField("paddings", paddings).Debug("calculating bound")
	for idx, padding := range paddings {
		if math.Abs(padding) < 1 {
			if idx == 0 || idx == 2 {
				paddings[idx] = padding * height
			} else {
				paddings[idx] = padding * width
			}
		}
	}
	x1 := x + width
	y1 := y + height
	logrus.WithField("x", x).WithField("y", y).WithField("x1", x1).WithField("y1", y1).Debug("calculating bound")

	if r.setting.Background.HThroughout {
		x = 0
		x1 = float64(dc.Width())
	} else {
		x = math.Max(x-paddings[3], 0)
		x1 = math.Min(x1+paddings[1], float64(dc.Width()))
	}
	if r.setting.Background.VThroughout {
		y = 0
		y1 = float64(dc.Height())
	} else {
		y = math.Max(y-paddings[0], 0)
		y1 = math.Min(y1+paddings[2], float64(dc.Height()))
	}

	bx = x
	by = y
	bw = x1 - x
	bh = y1 - y
	logrus.Debugf("bound: %v", map[string]float64{
		"bx": bx, "by": y,
		"bw": bw, "bh": bh,
	})
	return bx, by, bw, bh
}

func (r render) renderText(text string) error {
	alignMap := map[string]gg.Align{
		alignLeft:   gg.AlignLeft,
		alignCenter: gg.AlignCenter,
		alignRight:  gg.AlignRight,
	}
	align := r.setting.Alignment
	if _, ok := alignMap[align]; !ok {
		logrus.Warnf("invalid alignment %s, fallback to %s", align, fallbackAlign)
		align = fallbackAlign
	}

	x, y, ax, ay, width := r.position(text)
	if viper.GetBool("debug-rendering") {
		r.ctx.SetHexColor("ff00ff")
		r.ctx.SetLineWidth(5)
		r.ctx.DrawCircle(x, y, 10)
		r.ctx.SetLineWidth(3)
		r.ctx.DrawLine(x-10, y, x+10, y)
		r.ctx.DrawLine(x, y-10, x, y+10)
		r.ctx.Stroke()
	}
	r.loadTextColor()
	logrus.WithField("linespace", r.setting.Linespace).Debug("draw string wrapped")
	r.ctx.DrawStringWrapped(
		text,
		x, y,
		ax, ay,
		width,
		r.setting.Linespace,
		alignMap[align],
	)

	return nil
}

func (r render) renderBackground(text string) {
	x, y, ax, ay, width := r.position(text)
	bx, by, bw, bh := r.boundOf(text, x, y, ax, ay, width)
	r.ctx.DrawRectangle(bx, by, bw, bh)
	if viper.GetBool("debug-rendering") {
		r.ctx.SetHexColor("ff00ff")
		r.ctx.StrokePreserve()
	}
	r.loadBackgroundColor()
	r.ctx.Fill()
}

func (r render) image() image.Image {
	return r.ctx.Image()
}

func (r *render) updateSetting(setting watermarkSetting) {
	r.setting = setting
}

func (r render) sizeAfterFill() (w, h float64) {
	w = float64(r.ctx.Width())
	h = float64(r.ctx.Height())
	fillRatio := r.setting.ReferenceWidth / r.setting.ReferenceHeight
	logrus.WithField("ratio", w/h).WithField("w", r.setting.ReferenceWidth).WithField("h", r.setting.ReferenceHeight).Debug("ref size")
	switch ratio := w / h; {
	case ratio > fillRatio:
		// over width
		return h * fillRatio, h
	case ratio < fillRatio:
		// over height
		return w, w / fillRatio
	}
	// same ratio
	return w, h
}

func (r render) cutAfterFill() (horizontal, vertical float64) {
	w, h := r.sizeAfterFill()

	return (float64(r.ctx.Width()) - w) / 2, (float64(r.ctx.Height()) - h) / 2
}
