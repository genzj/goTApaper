package actor

import (
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

// ThumbnailQuality is the JPEG quality thumbnails are encoded at. Lower than
// the wallpaper's own 90: a thumbnail is a recognition aid in a list, not
// something anyone looks at closely.
const ThumbnailQuality = 80

// WriteThumbnail scales im down to at most width pixels wide and writes it to
// path as a JPEG, creating the directory if needed. It returns the path
// written.
//
// Thumbnails exist because the wallpaper file is overwritten on every refresh,
// so a history entry has nothing of its own to show without one. They are
// always JPEG regardless of the source format, since the point is to be small.
func WriteThumbnail(im image.Image, path string, width int) (string, error) {
	if im == nil {
		return "", fmt.Errorf("no image to thumbnail")
	}
	if width < 1 {
		return "", fmt.Errorf("invalid thumbnail width %d", width)
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("cannot create the thumbnail directory %s: %w", dir, err)
		}
	}

	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if err := jpeg.Encode(out, scaleToWidth(im, width), &jpeg.Options{
		Quality: ThumbnailQuality,
	}); err != nil {
		// Remove the partial file rather than leave something that decodes to
		// half a picture, which the UI would render as a broken image.
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// scaleToWidth returns im scaled to width, preserving the aspect ratio. An
// image already no wider than width is returned unchanged.
func scaleToWidth(im image.Image, width int) image.Image {
	bounds := im.Bounds()
	if bounds.Dx() <= width || bounds.Dx() == 0 {
		return im
	}

	height := bounds.Dy() * width / bounds.Dx()
	if height < 1 {
		height = 1
	}

	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	// CatmullRom rather than ApproxBiLinear: this runs once per refresh on an
	// image that is about to be discarded anyway, so quality is worth more
	// than the few milliseconds.
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), im, bounds, draw.Over, nil)
	return scaled
}
