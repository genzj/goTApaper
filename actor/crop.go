package actor

import (
	"image"
	"image/draw"
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
	case "yes":
	// handle below
	default:
		logrus.WithField("crop", option).Warn("unknown crop option")
		return im
	}

	bounds := im.Bounds()
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

	var cropped image.Image
	switch im := im.(type) {
	case *image.RGBA:
		cropped = im.SubImage(
			image.Rect(int(lCut), int(tCut), int(lCut+w), int(tCut+h)),
		)
	case *image.NRGBA:
		cropped = im.SubImage(
			image.Rect(int(lCut), int(tCut), int(lCut+w), int(tCut+h)),
		)
	case *image.YCbCr:
		cropped = im.SubImage(
			image.Rect(int(lCut), int(tCut), int(lCut+w), int(tCut+h)),
		)
	default:
		logrus.Warnf("invalid image format %T", im)
		cropped = im
	}

	bounds = cropped.Bounds()
	newImg := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	// Copy the subimage to the new image, translating coordinates
	draw.Draw(newImg, newImg.Bounds(), cropped, bounds.Min, draw.Src)

	return newImg
}

// DefaultCropper for convenience
var DefaultCropper Cropper
