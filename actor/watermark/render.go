package watermark

import (
	"image"
	"math"
	"strings"

	"github.com/sirupsen/logrus"

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
	fallbackColor    = "00000080"
	fallbackPosition = positionBottomRight
	fallbackAlign    = alignRight
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

func (r render) loadColor() {
	// in case the color in configuration not work
	r.ctx.SetHexColor(fallbackColor)
	r.ctx.SetHexColor(r.setting.Color)
}
func (r render) loadFont() error {
	pixelDense := float64(r.ctx.Height() / 1080.0)
	fontFile, err := findFont(r.setting.Font)
	if err != nil {
		logrus.WithError(err).Errorf("cannot find font %s", r.setting.Font)
		return err
	}
	err = r.ctx.LoadFontFace(fontFile, math.Round(float64(r.setting.Point)*pixelDense))
	if err != nil {
		logrus.WithError(err).Errorf("cannot load font %s", fontFile)
		return err
	}

	return nil
}

func (r render) renderText(text string) error {
	var maxLineWidth float64 = 0
	vPad := r.setting.VPadding
	hPad := r.setting.HPadding

	ratioDense := float64(r.ctx.Width()/r.ctx.Height()) / (1920.0 / 1080.0)

	if vPad <= 0 {
		vPad = 0.05
	}
	if hPad <= 0 {
		hPad = 0.05
	}

	if vPad < 1 {
		vPad *= float64(r.ctx.Height())
	}

	if hPad < 1 {
		hPad *= float64(r.ctx.Width())
	}

	hPad *= ratioDense
	vPad /= ratioDense

	topOffset := func() float64 {
		return vPad
	}
	bottomOffset := func() float64 {
		return float64(r.ctx.Height()) - vPad
	}
	leftOffset := func() float64 {
		return hPad
	}
	rightOffset := func() float64 {
		return float64(r.ctx.Width()) - hPad
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
	alignMap := map[string]gg.Align{
		alignLeft:   gg.AlignLeft,
		alignCenter: gg.AlignCenter,
		alignRight:  gg.AlignRight,
	}

	position := r.setting.Position
	if _, ok := xCalculator[position]; !ok {
		logrus.Warnf("invalid position %s, fallback to %s", position, fallbackPosition)
		position = fallbackPosition
	}

	x := xCalculator[position]()
	y := yCalculator[position]()
	a := aCalculator[position]
	ax, ay := a[0], a[1]
	logrus.Debugf("watermark position: %##v", map[string]float64{
		"x": x, "y": y,
		"ax": ax, "ay": ay,
	})

	for _, line := range strings.Split(text, "\n") {
		if lineW, _ := r.ctx.MeasureString(line); lineW > maxLineWidth {
			maxLineWidth = lineW
		}
	}

	align := r.setting.Alignment
	if _, ok := alignMap[align]; !ok {
		logrus.Warnf("invalid alignment %s, fallback to %s", align, fallbackAlign)
		align = fallbackAlign
	}
	r.ctx.DrawStringWrapped(
		text,
		x, y,
		ax, ay,
		maxLineWidth,
		r.setting.Linespace,
		alignMap[align],
	)

	return nil
}

func (r render) image() image.Image {
	return r.ctx.Image()
}
