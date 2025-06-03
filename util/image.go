package util

import (
	"bytes"
	"image"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Viewpoint returns the visible region of a image after being centered and
// filling the desktop
func Viewpoint(w0, h0 float64) (w, h float64) {
	refWidth := viper.GetFloat64("reference-width")
	refHeight := viper.GetFloat64("reference-height")
	fillRatio := refWidth / refHeight

	w1 := w0
	h1 := h0

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

// DecodeFromResponse return picture in http response
func DecodeFromResponse(resp *http.Response) (raw *bytes.Reader, img image.Image, format string, err error) {
	defer func() {
		_ = resp.Body.Close()
	}()

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, "", err
	}

	raw = bytes.NewReader(bs)
	logrus.WithField("filesize", raw.Len()).Info("wallpaper downloaded")

	reader2 := bytes.NewReader(bs)
	img, format, err = image.Decode(reader2)
	if err != nil {
		return raw, nil, "", err
	}
	return raw, img, format, nil
}

// DecodeFromFile returns picture from a file path
func DecodeFromFile(filepath string) (raw *bytes.Reader, img image.Image, format string, err error) {
	if runtime.GOOS == "windows" {
		filepath, _ = strings.CutPrefix(filepath, "/")
	}

	bs, err := os.ReadFile(filepath)
	if err != nil {
		return nil, nil, "", err
	}

	raw = bytes.NewReader(bs)
	logrus.WithField("filesize", raw.Len()).WithField("filepath", filepath).Info("file loaded")

	reader2 := bytes.NewReader(bs)
	img, format, err = image.Decode(reader2)
	if err != nil {
		return raw, nil, "", err
	}
	return raw, img, format, nil
}
