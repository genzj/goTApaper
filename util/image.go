package util

import (
	"math"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Viewpoint returns the visible region of a image after being centered and
// filling the desktop
func Viewpoint(w0, h0 float64) (w, h float64) {
	refWidth := viper.GetFloat64("reference-width")
	refHeight := viper.GetFloat64("reference-height")
	fillRatio := refWidth / refHeight

	w1 := float64(w0)
	h1 := float64(h0)

	logger := logrus.WithFields(map[string]interface{}{
		"w0":        w0,
		"h0":        h0,
		"fillRatio": fillRatio,
		"refWidth":  refWidth,
		"refHeight": refHeight,
		"ratio":     w1 / h1,
		"w1":        w1,
		"h1":        h1,
	})
	if math.IsInf(fillRatio, 0) || math.IsNaN(fillRatio) || fillRatio == 0 {
		logger.Warn("invalid refence width or height, use original picture")
	} else {
		switch ratio := w1 / h1; {
		case ratio > fillRatio:
			// over width
			w1 = h1 * fillRatio
		case ratio < fillRatio:
			// over height
			h1 = w1 / fillRatio
		case ratio == fillRatio:
			// same ratio, no change
		}
	}
	logger.WithField("w1", w1).WithField("h1", h1).Debug("viewpoint located")

	return w1, h1
}
