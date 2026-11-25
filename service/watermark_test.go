package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/config/store/sqlite"
)

func TestDownloaderPreviewAndRevert(t *testing.T) {
	d, f, _ := newTestDownloader(t)

	if err := d.ShowPreview("test"); err == nil {
		t.Fatal("ShowPreview without preview functions should fail")
	}

	var reverted int
	d.SetPreview(
		func() (int64, error) { return 7, nil },
		func() error { reverted++; return nil },
	)

	if err := d.ShowPreview("test"); err != nil {
		t.Fatalf("ShowPreview: %v", err)
	}
	waitFor(t, "idle", idle(d))
	if st := d.Status(); !st.Previewing || st.PreviewRevision != 7 {
		t.Fatalf("Status = %+v, want previewing revision 7", st)
	}
	if st := d.Status(); st.Last == nil || st.Last.Job.Kind != JobPreview {
		t.Errorf("last job = %+v, want the preview", st.Last)
	}

	// A preview is refused while a download runs, like any user request.
	if err := d.Refresh(nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if err := d.ShowPreview("test"); !errors.Is(err, ErrBusy) {
		t.Errorf("ShowPreview while busy = %v, want ErrBusy", err)
	}
	// RevertPreviewNow does not barge in on a running job either.
	if err := d.RevertPreviewNow(); err != nil || reverted != 0 {
		t.Errorf("RevertPreviewNow while busy = %v, reverted %d times", err, reverted)
	}

	// A refresh that applies a picture replaces the preview.
	close(f.release)
	waitFor(t, "idle", idle(d))
	if d.Status().Previewing {
		t.Error("still previewing after a refresh applied a picture")
	}

	if err := d.ShowPreview("test"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "idle", idle(d))
	if err := d.RevertPreview("test"); err != nil {
		t.Fatalf("RevertPreview: %v", err)
	}
	waitFor(t, "idle", idle(d))
	if d.Status().Previewing || reverted != 1 {
		t.Errorf("after revert: previewing %v, reverted %d times", d.Status().Previewing, reverted)
	}

	if err := d.ShowPreview("test"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "idle", idle(d))
	if err := d.RevertPreviewNow(); err != nil || reverted != 2 || d.Previewing() {
		t.Errorf("RevertPreviewNow = %v, reverted %d times, previewing %v", err, reverted, d.Previewing())
	}
}

func newWatermarksService(t *testing.T) (*Watermarks, store.Store) {
	t.Helper()
	config.SetAppDir(t.TempDir())
	t.Cleanup(func() { config.SetAppDir("") })

	s, err := sqlite.Open(sqlite.MemoryPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	w := NewWatermarks(s)
	w.crop = func(im image.Image) image.Image { return im }
	return w, s
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, im, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWatermarksCaptureAndRender(t *testing.T) {
	w, s := newWatermarksService(t)

	if _, err := w.Capture(); !errors.Is(err, ErrNoCapture) {
		t.Fatalf("Capture with no history = %v, want ErrNoCapture", err)
	}

	// A refresh keeps the original, then records the entry.
	meta := &channel.PictureMeta{Title: "Lake", Format: "jpeg", URL: "https://example.invalid/a.jpg"}
	RecordOriginal(bytes.NewReader(jpegBytes(t, 320, 200)), meta)
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "bing", URL: meta.URL, Title: meta.Title, Format: "jpeg",
		FilePath: filepath.Join(config.AppDir(), "wallpaper.jpeg"), DownloadTime: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	base, err := w.Capture()
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if base.Width != 320 || base.Height != 200 || base.Title != "Lake" || base.Stale {
		t.Errorf("base = %+v", base)
	}

	layer := store.NewWatermarkLayer("NoSuchFont.ttf")
	res, err := w.Render(context.Background(), []store.WatermarkLayer{layer}, false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if res.Revision != 1 || res.Width != 320 || len(res.Layers) != 1 {
		t.Errorf("result = %+v", res)
	}
	if e := res.Layers[0].Error; e == nil || e.Stage != "font" {
		t.Errorf("missing font not reported: %+v", res.Layers[0])
	}

	path, rev, err := w.PreviewFile()
	if err != nil || rev != 1 {
		t.Fatalf("PreviewFile = %s, %d, %v", path, rev, err)
	}
	if filepath.Dir(path) != config.GetPreviewDir() {
		t.Errorf("preview written to %s, want the preview directory", path)
	}
	// The real wallpaper file is never written by a preview.
	if _, err := os.Stat(filepath.Join(config.AppDir(), "wallpaper.jpeg")); !os.IsNotExist(err) {
		t.Error("the preview wrote the wallpaper file")
	}

	img, rev, err := w.Image(160)
	if err != nil || rev != 1 {
		t.Fatalf("Image = %v, %d", err, rev)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(img))
	if err != nil || cfg.Width != 160 || cfg.Height != 100 {
		t.Errorf("scaled image = %+v, %v; want 160x100", cfg, err)
	}

	// A sample leaves the preview alone.
	sample, err := w.Sample([]store.WatermarkLayer{store.NewWatermarkLayer("x.ttf")}, 100)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(sample)); err != nil || cfg.Width != 100 {
		t.Errorf("sample = %+v, %v", cfg, err)
	}
	if _, rev, _ := w.PreviewFile(); rev != 1 {
		t.Errorf("a sample changed the preview revision to %d", rev)
	}

	// A newer wallpaper makes the capture stale.
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "bing", URL: "https://example.invalid/b.jpg", DownloadTime: 2000,
	}); err != nil {
		t.Fatal(err)
	}
	if base, _ := w.Base(); !base.Stale {
		t.Error("capture not reported stale after a newer wallpaper")
	}
}

func TestValidateLayersChecksTemplates(t *testing.T) {
	l := store.NewWatermarkLayer("x.ttf")
	l.Template = "{{.Title"
	var invalid *config.InvalidValueError
	if err := ValidateLayers([]store.WatermarkLayer{l}); !errors.As(err, &invalid) {
		t.Errorf("ValidateLayers = %v, want InvalidValueError", err)
	}
	l.Template = `{{date "2006" .UploadTime}}`
	if err := ValidateLayers([]store.WatermarkLayer{l}); err != nil {
		t.Errorf("ValidateLayers = %v", err)
	}
}
