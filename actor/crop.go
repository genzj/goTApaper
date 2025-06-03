package actor

import (
	"image"
	"runtime"

	"github.com/genzj/goTApaper/util"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Cropper crops picture according to configured reference ratio
type Cropper int

// Crop picture
func (Cropper) Crop(im image.Image) image.Image {
	switch option := viper.GetString("crop"); option {
	case "no":
		return im
	case "win-only":
		if runtime.GOOS != "windows" {
			return im
		}
	case "force":
	// handle below
	default:
		logrus.WithField("crop", option).Warn("unknown crop option")
		return im
	}

	if rgba, ok := im.(*image.RGBA); ok {
		bounds := rgba.Bounds()
		w, h := util.Viewpoint(float64(bounds.Dx()), float64(bounds.Dy()))
		tCut := (float64(bounds.Dy()) - h) / 2
		lCut := (float64(bounds.Dx()) - w) / 2
		logrus.WithField(
			"h", h,
		).WithField(
			"w", w,
		).WithField(
			"x", lCut,
		).WithField(
			"y", tCut,
		).Debug("pos after cut")
		cropped := rgba.SubImage(
			image.Rect(int(lCut), int(tCut), int(lCut+w), int(tCut+h)),
		)
		return cropped
	}
	logrus.WithField("image", im).Warn("invalid image format")
	return im
}

// DefaultCropper for convenience
var DefaultCropper Cropper
