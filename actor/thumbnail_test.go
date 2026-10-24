package actor

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int) image.Image {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			im.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	return im
}

func decode(t *testing.T, path string) image.Image {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	defer file.Close()

	im, err := jpeg.Decode(file)
	if err != nil {
		t.Fatalf("the thumbnail is not decodable JPEG: %v", err)
	}
	return im
}

func TestWriteThumbnailScalesDownAndPreservesTheAspectRatio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumb.jpg")

	if _, err := WriteThumbnail(solid(1920, 1080), path, 480); err != nil {
		t.Fatalf("WriteThumbnail: %v", err)
	}

	bounds := decode(t, path).Bounds()
	if bounds.Dx() != 480 {
		t.Errorf("width = %d, want 480", bounds.Dx())
	}
	// 1080 * 480 / 1920 = 270.
	if bounds.Dy() != 270 {
		t.Errorf("height = %d, want 270", bounds.Dy())
	}
}

func TestWriteThumbnailCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumbs", "nested", "thumb.jpg")

	written, err := WriteThumbnail(solid(100, 100), path, 480)
	if err != nil {
		t.Fatalf("WriteThumbnail: %v", err)
	}
	if written != path {
		t.Errorf("returned %q, want %q", written, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the thumbnail was not written: %v", err)
	}
}

func TestWriteThumbnailLeavesASmallImageAtItsOwnSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumb.jpg")

	// Upscaling a small picture would only waste bytes.
	if _, err := WriteThumbnail(solid(200, 100), path, 480); err != nil {
		t.Fatalf("WriteThumbnail: %v", err)
	}

	bounds := decode(t, path).Bounds()
	if bounds.Dx() != 200 || bounds.Dy() != 100 {
		t.Fatalf("size = %dx%d, want 200x100", bounds.Dx(), bounds.Dy())
	}
}

func TestWriteThumbnailIsMuchSmallerThanTheSource(t *testing.T) {
	dir := t.TempDir()
	thumb := filepath.Join(dir, "thumb.jpg")

	if _, err := WriteThumbnail(solid(1920, 1080), thumb, 480); err != nil {
		t.Fatalf("WriteThumbnail: %v", err)
	}

	info, err := os.Stat(thumb)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// The entry cap assumes thumbnails are tens of kilobytes, not megabytes;
	// this is what keeps the directory bounded at roughly 20 MiB.
	if info.Size() > 200*1024 {
		t.Fatalf("thumbnail is %d bytes, larger than expected for 480px", info.Size())
	}
}

func TestWriteThumbnailRejectsBadInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumb.jpg")

	if _, err := WriteThumbnail(nil, path, 480); err == nil {
		t.Error("WriteThumbnail accepted a nil image")
	}
	if _, err := WriteThumbnail(solid(10, 10), path, 0); err == nil {
		t.Error("WriteThumbnail accepted a zero width")
	}
}

func TestWriteThumbnailHandlesAnExtremeAspectRatio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumb.jpg")

	// A height that would round to zero must still produce a valid image
	// rather than an encoder error.
	if _, err := WriteThumbnail(solid(10000, 1), path, 480); err != nil {
		t.Fatalf("WriteThumbnail: %v", err)
	}
	if got := decode(t, path).Bounds().Dy(); got != 1 {
		t.Fatalf("height = %d, want 1", got)
	}
}
