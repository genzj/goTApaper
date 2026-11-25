package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/image/draw"

	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/actor/watermark"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/util"
)

// ErrNoCapture is returned when the editor asks for a preview before a picture
// has been captured, and there is nothing to capture.
var ErrNoCapture = errors.New("no wallpaper has been applied yet; refresh once to enable previews")

// ErrSuperseded is returned by a preview render that a newer one replaced
// before it started. The client that asked has moved on.
var ErrSuperseded = errors.New("superseded by a newer preview")

const (
	originalName     = "current"
	originalMetaName = "current.json"
	previewBaseName  = "base"
	previewMetaName  = "base.json"
	previewFileName  = "preview.jpg"

	// previewQuality is the JPEG quality of the rendered preview file, which
	// the desktop preview sets as is, so it matches the refresh's.
	previewQuality = 90
	// previewImageQuality is the quality of the scaled copies the editor
	// shows, which are re-made on every render.
	previewImageQuality = 85
	// MaxPreviewImageWidth caps the scaled copy the editor asks for.
	MaxPreviewImageWidth = 8192
)

// pictureRecord is what is stored next to a saved picture: enough to show its
// metadata in templates and to tell whether it is still the newest one.
type pictureRecord struct {
	Format string              `json:"format"`
	Meta   channel.PictureMeta `json:"meta"`
	// EntryID is the history entry it was captured from, zero for the
	// original cache, which is written before the entry is.
	EntryID int64 `json:"entryId,omitempty"`
	SavedAt int64 `json:"savedAt"`
}

// RecordOriginal keeps the picture a refresh is applying, as downloaded,
// before it is cropped and watermarked: the wallpaper file has the watermark
// burnt in, so it cannot be edited again, and a provider's link may expire.
// One picture is kept, replacing the last.
//
// Failures are logged rather than returned, like the history's: the
// wallpaper is still worth applying without it.
func RecordOriginal(raw *bytes.Reader, meta *channel.PictureMeta) {
	if raw == nil || meta == nil {
		return
	}
	dir := config.GetOriginalDir()
	l := logrus.WithField("dir", dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		l.WithError(err).Warn("cannot create the original-picture directory")
		return
	}

	format := strings.ToLower(meta.Format)
	if format == "" {
		format = "jpeg"
	}
	// Read through a section so the caller's reader is left where it was.
	section := io.NewSectionReader(raw, 0, raw.Size())
	path := filepath.Join(dir, originalName+"."+format)
	if err := writeAtomically(path, func(w io.Writer) error {
		_, err := io.Copy(w, section)
		return err
	}); err != nil {
		l.WithError(err).Warn("cannot keep the original picture")
		return
	}
	removeSiblings(dir, originalName+".", path)

	rec := pictureRecord{Format: format, Meta: *meta, SavedAt: time.Now().Unix()}
	if err := writeJSON(filepath.Join(dir, originalMetaName), rec); err != nil {
		l.WithError(err).Warn("cannot record the original picture's metadata")
	}
}

// MetaFromEntry rebuilds a picture's metadata from its history entry, for the
// watermark templates, which have to show the original's title and credit.
func MetaFromEntry(e store.WallpaperHistoryEntry, format string) *channel.PictureMeta {
	meta := &channel.PictureMeta{
		Title:        e.Title,
		Caption:      e.Caption,
		Credit:       e.Credit,
		Format:       format,
		Channel:      e.ChannelType,
		ChannelKey:   e.ChannelName,
		URL:          e.URL,
		DownloadTime: time.Unix(e.DownloadTime, 0),
	}
	if e.UploadTime != 0 {
		meta.UploadTime = time.Unix(e.UploadTime, 0)
	}
	return meta
}

// PreviewBase describes the picture the editor is working on.
type PreviewBase struct {
	EntryID    int64  `json:"entryId,omitempty"`
	Title      string `json:"title,omitempty"`
	URL        string `json:"url,omitempty"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	CapturedAt int64  `json:"capturedAt"`
	// Stale reports that a newer wallpaper has been applied since the
	// capture, so the editor can offer to capture again.
	Stale bool `json:"stale"`
}

// PreviewResult is one rendered preview.
type PreviewResult struct {
	// Revision increases with every render, so a client can tell the image
	// it holds from the latest and the desktop can be compared with both.
	Revision int64                   `json:"revision"`
	Width    int                     `json:"width"`
	Height   int                     `json:"height"`
	Layers   []watermark.LayerResult `json:"layers"`
	Base     PreviewBase             `json:"base"`
}

// TemplateHelp is the template reference the editor's completion and help
// use.
type TemplateHelp struct {
	Fields []watermark.TemplateField `json:"fields"`
	Funcs  []watermark.TemplateFunc  `json:"funcs"`
}

// Watermarks is the watermark editor's backend: fonts, presets, the template
// reference, and the preview.
//
// A preview is one file: the captured picture rendered with the draft layers
// at full size. The editor shows a scaled copy of it and the desktop preview
// sets the file itself, so what the user sees in the window and on the
// desktop is the same render. Nothing here downloads, writes the wallpaper
// file or touches the history.
type Watermarks struct {
	store store.Store
	// crop is the refresh's cropper, a field so tests can leave pictures
	// whole without configuring one.
	crop func(image.Image) image.Image

	mu sync.Mutex
	// generation counts render requests, so one superseded while it waited
	// for the lock can give up.
	generation int64

	base      image.Image // cropped captured picture, decoded once
	baseInfo  PreviewBase
	baseMeta  *channel.PictureMeta
	baseStamp time.Time

	rendered image.Image
	revision int64
	scaled   map[int][]byte // revision's scaled copies, by width
}

// NewWatermarks builds the watermark editor service.
func NewWatermarks(s store.Store) *Watermarks {
	return &Watermarks{store: s, crop: actor.DefaultCropper.Crop}
}

// Fonts lists the fonts layers can use.
//
// The list is built once per process; restarting goTApaper picks up fonts
// installed since.
func (w *Watermarks) Fonts() []watermark.FontInfo {
	return watermark.Fonts()
}

// UnsupportedFonts lists installed fonts the renderer cannot load, and why.
func (w *Watermarks) UnsupportedFonts() []watermark.UnsupportedFont {
	return watermark.UnsupportedFonts()
}

// FontFile returns the path of an indexed font, by id.
func (w *Watermarks) FontFile(id string) (string, error) {
	f, ok := watermark.FontByID(id)
	if !ok {
		return "", fmt.Errorf("%w: font %q", store.ErrNotFound, id)
	}
	return f.Path, nil
}

// Presets lists the built-in presets.
func (w *Watermarks) Presets() []watermark.Preset {
	return watermark.Presets()
}

// TemplateHelp returns the template reference, with each field's value on the
// captured picture when there is one.
func (w *Watermarks) TemplateHelp() TemplateHelp {
	w.mu.Lock()
	meta := w.baseMeta
	w.mu.Unlock()
	if meta == nil {
		if rec, err := readRecord(filepath.Join(config.GetPreviewDir(), previewMetaName)); err == nil {
			meta = &rec.Meta
		}
	}
	return TemplateHelp{Fields: watermark.FieldsFor(meta), Funcs: watermark.TemplateFuncs}
}

// ValidateLayers checks a list of layers, templates included, as a save or a
// preview would.
func ValidateLayers(layers []store.WatermarkLayer) error {
	if len(layers) > store.MaxWatermarkLayers {
		return &config.InvalidValueError{
			Key: "watermark", Value: fmt.Sprint(len(layers)),
			Kind: config.Kind(fmt.Sprintf("list of at most %d layers", store.MaxWatermarkLayers)),
		}
	}
	for i, l := range layers {
		if err := store.ValidateWatermarkLayer(l); err != nil {
			return fmt.Errorf("layer %d: %w", i+1, err)
		}
		if terr := watermark.CheckTemplate(l.Template); terr != nil {
			return fmt.Errorf("layer %d: %w", i+1, &config.InvalidValueError{
				Key: "watermark.template", Value: l.Template, Kind: config.Kind("valid template (" + terr.Message + ")"),
			})
		}
	}
	return nil
}

// Capture copies the newest wallpaper's original picture into the preview
// directory, so the editor keeps working on the same picture even if a refresh
// replaces the wallpaper meanwhile.
//
// The original kept by RecordOriginal is used when it belongs to the newest
// entry. Failing that -- the wallpaper was applied by a version that kept no
// original -- the entry's URL is fetched again; a provider may have expired
// it, in which case ErrNoCapture says to refresh once.
func (w *Watermarks) Capture() (PreviewBase, error) {
	entries, err := w.store.WallpaperHistory(1, 0)
	if err != nil {
		return PreviewBase{}, err
	}
	if len(entries) == 0 {
		return PreviewBase{}, ErrNoCapture
	}
	entry := entries[0]

	dir := config.GetPreviewDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PreviewBase{}, err
	}

	var raw []byte
	format := ""
	if rec, err := readRecord(filepath.Join(config.GetOriginalDir(), originalMetaName)); err == nil &&
		entry.URL != "" && rec.Meta.URL == entry.URL {
		raw, err = os.ReadFile(filepath.Join(config.GetOriginalDir(), originalName+"."+rec.Format))
		if err == nil {
			format = rec.Format
		}
	}
	if raw == nil {
		if entry.URL == "" {
			return PreviewBase{}, ErrNoCapture
		}
		logrus.WithField("url", entry.URL).Info("no original kept for the current wallpaper; downloading it for the editor")
		reader, _, f, err := util.FetchPicture(entry.URL)
		if err != nil {
			return PreviewBase{}, fmt.Errorf("%w (downloading it again failed: %v)", ErrNoCapture, err)
		}
		format = f
		if raw, err = io.ReadAll(reader); err != nil {
			return PreviewBase{}, err
		}
	}

	basePath := filepath.Join(dir, previewBaseName+"."+format)
	if err := writeAtomically(basePath, func(out io.Writer) error {
		_, err := out.Write(raw)
		return err
	}); err != nil {
		return PreviewBase{}, err
	}
	removeSiblings(dir, previewBaseName+".", basePath)

	rec := pictureRecord{
		Format:  format,
		Meta:    *MetaFromEntry(entry, format),
		EntryID: entry.ID,
		SavedAt: time.Now().Unix(),
	}
	if err := writeJSON(filepath.Join(dir, previewMetaName), rec); err != nil {
		return PreviewBase{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.base = nil // decoded again, with the current crop setting, on next use
	if err := w.loadBaseLocked(); err != nil {
		return PreviewBase{}, err
	}
	return w.baseInfoLocked(), nil
}

// Base describes the captured picture, capturing one if there is none.
func (w *Watermarks) Base() (PreviewBase, error) {
	w.mu.Lock()
	err := w.loadBaseLocked()
	info := w.baseInfoLocked()
	w.mu.Unlock()
	if errors.Is(err, ErrNoCapture) {
		return w.Capture()
	}
	return info, err
}

// loadBaseLocked decodes and crops the captured picture, unless it is already
// in memory and unchanged on disk.
func (w *Watermarks) loadBaseLocked() error {
	dir := config.GetPreviewDir()
	rec, err := readRecord(filepath.Join(dir, previewMetaName))
	if err != nil {
		return ErrNoCapture
	}
	path := filepath.Join(dir, previewBaseName+"."+rec.Format)
	info, err := os.Stat(path)
	if err != nil {
		return ErrNoCapture
	}
	if w.base != nil && info.ModTime().Equal(w.baseStamp) {
		return nil
	}

	_, img, _, err := util.DecodeFromFile(path)
	if err != nil {
		return fmt.Errorf("cannot read the captured picture: %w", err)
	}
	// Cropped as a refresh would, so the preview matches what applying the
	// layers would produce.
	img = w.crop(img)

	w.base = img
	w.baseStamp = info.ModTime()
	w.baseMeta = &rec.Meta
	b := img.Bounds()
	w.baseInfo = PreviewBase{
		EntryID:    rec.EntryID,
		Title:      rec.Meta.Title,
		URL:        rec.Meta.URL,
		Width:      b.Dx(),
		Height:     b.Dy(),
		CapturedAt: rec.SavedAt,
	}
	return nil
}

func (w *Watermarks) baseInfoLocked() PreviewBase {
	info := w.baseInfo
	if entries, err := w.store.WallpaperHistory(1, 0); err == nil && len(entries) > 0 {
		info.Stale = entries[0].ID != info.EntryID || entries[0].URL != info.URL
	}
	return info
}

// Render draws layers onto the captured picture at full size and writes the
// result to the preview file. debug draws the rendering debug overlay into it.
//
// Renders run one at a time. One that a newer request superseded while it
// waited returns ErrSuperseded without rendering, so dragging a layer does not
// queue a render per mouse move.
func (w *Watermarks) Render(ctx context.Context, layers []store.WatermarkLayer, debug bool) (PreviewResult, error) {
	if len(layers) > store.MaxWatermarkLayers {
		return PreviewResult{}, ValidateLayers(layers)
	}
	for i, l := range layers {
		// The template is not checked here: its error is part of the
		// result, with a position the editor can underline.
		if err := store.ValidateWatermarkLayer(l); err != nil {
			return PreviewResult{}, fmt.Errorf("layer %d: %w", i+1, err)
		}
	}

	w.mu.Lock()
	w.generation++
	mine := w.generation
	w.mu.Unlock()

	if _, err := w.Base(); err != nil {
		return PreviewResult{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if mine != w.generation || ctx.Err() != nil {
		return PreviewResult{}, ErrSuperseded
	}
	if err := w.loadBaseLocked(); err != nil {
		return PreviewResult{}, err
	}

	out, results := watermark.RenderLayers(w.base, w.baseMeta, layers, watermark.RenderOptions{Debug: debug})

	path := filepath.Join(config.GetPreviewDir(), previewFileName)
	if err := writeAtomically(path, func(f io.Writer) error {
		return jpeg.Encode(f, out, &jpeg.Options{Quality: previewQuality})
	}); err != nil {
		return PreviewResult{}, fmt.Errorf("cannot write the preview: %w", err)
	}

	w.rendered = out
	w.revision++
	w.scaled = map[int][]byte{}

	b := out.Bounds()
	return PreviewResult{
		Revision: w.revision,
		Width:    b.Dx(),
		Height:   b.Dy(),
		Layers:   results,
		Base:     w.baseInfoLocked(),
	}, nil
}

// Image returns the latest render scaled to width, as a JPEG, and its
// revision. A width of zero or more than the picture returns it full size.
func (w *Watermarks) Image(width int) ([]byte, int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.rendered == nil {
		return nil, 0, fmt.Errorf("%w: nothing has been previewed yet", store.ErrNotFound)
	}
	if width > MaxPreviewImageWidth {
		width = MaxPreviewImageWidth
	}
	b := w.rendered.Bounds()
	if width <= 0 || width > b.Dx() {
		width = b.Dx()
	}
	if cached, ok := w.scaled[width]; ok {
		return cached, w.revision, nil
	}

	src := w.rendered
	if width != b.Dx() {
		height := max(1, b.Dy()*width/b.Dx())
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
		src = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: previewImageQuality}); err != nil {
		return nil, 0, err
	}
	// A zoom asks for several large widths; keep only the latest few.
	if len(w.scaled) >= 4 {
		w.scaled = map[int][]byte{}
	}
	w.scaled[width] = buf.Bytes()
	return buf.Bytes(), w.revision, nil
}

// Sample renders layers onto a scaled-down copy of the captured picture and
// returns it as a JPEG, without touching the preview file or its revision --
// for showing presets side by side. Picture-relative layers render the same
// at any size, so the sample is faithful, only smaller.
func (w *Watermarks) Sample(layers []store.WatermarkLayer, width int) ([]byte, error) {
	if err := ValidateLayers(layers); err != nil {
		return nil, err
	}
	if _, err := w.Base(); err != nil {
		return nil, err
	}
	if width <= 0 || width > 960 {
		width = 480
	}

	w.mu.Lock()
	if err := w.loadBaseLocked(); err != nil {
		w.mu.Unlock()
		return nil, err
	}
	base, meta := w.base, w.baseMeta
	w.mu.Unlock()

	b := base.Bounds()
	if width > b.Dx() {
		width = b.Dx()
	}
	small := image.NewRGBA(image.Rect(0, 0, width, max(1, b.Dy()*width/b.Dx())))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), base, b, draw.Src, nil)

	out, _ := watermark.RenderLayers(small, meta, layers, watermark.RenderOptions{})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: previewImageQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PreviewFile returns the rendered preview file and its revision, for setting
// on the desktop.
func (w *Watermarks) PreviewFile() (string, int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	path := filepath.Join(config.GetPreviewDir(), previewFileName)
	if w.rendered == nil {
		return "", 0, fmt.Errorf("%w: render a preview first", store.ErrNotFound)
	}
	if _, err := os.Stat(path); err != nil {
		return "", 0, fmt.Errorf("%w: the preview file is gone; render it again", store.ErrNotFound)
	}
	return path, w.revision, nil
}

func readRecord(path string) (pictureRecord, error) {
	var rec pictureRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, err
	}
	if rec.Format == "" || strings.ContainsAny(rec.Format, `/\.`) {
		return rec, fmt.Errorf("%s names no usable format", path)
	}
	return rec, nil
}

func writeJSON(path string, v any) error {
	return writeAtomically(path, func(w io.Writer) error {
		return json.NewEncoder(w).Encode(v)
	})
}

// writeAtomically writes a file through a temporary sibling and a rename, so
// a reader -- the macOS setter copying the preview, the editor decoding the
// base -- never sees half a file.
func writeAtomically(path string, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := write(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// os.Rename replaces an existing file on every platform, Windows included.
	return os.Rename(tmp.Name(), path)
}

// removeSiblings deletes the files in dir starting with prefix other than
// keep: the copy of a picture kept in a format it no longer has.
func removeSiblings(dir, prefix, keep string) {
	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*"))
	for _, m := range matches {
		if m != keep && !strings.HasSuffix(m, ".json") {
			_ = os.Remove(m)
		}
	}
}
