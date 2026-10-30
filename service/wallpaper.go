package service

import (
	"fmt"
	"image"

	// Registered for their DecodeConfig, so reporting a wallpaper's
	// dimensions works for whatever format a channel served.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/genzj/goTApaper/config/store"
)

// Wallpaper exposes the applied wallpaper and its history. Downloading a new
// one goes through Downloader, which owns the one-at-a-time rule.
type Wallpaper struct {
	store store.Store
}

// NewWallpaper builds a wallpaper service.
func NewWallpaper(s store.Store) *Wallpaper {
	return &Wallpaper{store: s}
}

// LastApplied describes the wallpaper goTApaper applied most recently: the
// newest history entry, plus what is actually on disk.
//
// It is deliberately not called "current". goTApaper cannot see the desktop,
// only what it last set there, and the user may have changed the wallpaper by
// hand since.
type LastApplied struct {
	// Entry is the newest history row, absent if nothing has been applied yet.
	Entry *store.WallpaperHistoryEntry `json:"entry,omitempty"`
	// Path is the wallpaper file on disk.
	Path string `json:"path,omitempty"`
	// Size is its size in bytes, zero when the file is missing.
	Size int64 `json:"size,omitempty"`
	// Width and Height are its pixel dimensions, zero when undecodable.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// Exists reports whether Path is readable. A history entry whose file has
	// since been replaced or removed is still worth showing, so this is
	// reported rather than being an error.
	Exists bool `json:"exists"`
}

// LastApplied returns the wallpaper goTApaper applied most recently.
func (w *Wallpaper) LastApplied() (LastApplied, error) {
	out := LastApplied{}

	entries, err := w.store.WallpaperHistory(1, 0)
	if err != nil {
		return out, err
	}
	if len(entries) > 0 {
		out.Entry = &entries[0]
		out.Path = entries[0].FilePath
	}

	if out.Path == "" {
		return out, nil
	}

	info, err := os.Stat(out.Path)
	if err != nil {
		return out, nil
	}
	out.Exists = true
	out.Size = info.Size()

	file, err := os.Open(out.Path)
	if err != nil {
		return out, nil
	}
	defer file.Close()

	// DecodeConfig reads only the header, so this does not pay for decoding a
	// multi-megapixel photograph just to report its dimensions.
	if cfg, _, err := image.DecodeConfig(file); err == nil {
		out.Width = cfg.Width
		out.Height = cfg.Height
	}
	return out, nil
}

// LastAppliedImage opens the wallpaper file goTApaper applied most recently,
// for streaming to a client. The caller closes it.
func (w *Wallpaper) LastAppliedImage() (*os.File, string, error) {
	last, err := w.LastApplied()
	if err != nil {
		return nil, "", err
	}
	if last.Path == "" {
		return nil, "", fmt.Errorf("%w: no wallpaper has been applied yet", store.ErrNotFound)
	}

	file, err := os.Open(last.Path)
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf(
			"%w: the wallpaper file %s is no longer on disk", store.ErrNotFound, last.Path,
		)
	}
	if err != nil {
		return nil, "", err
	}

	contentType := "application/octet-stream"
	if last.Entry != nil {
		switch last.Entry.Format {
		case "jpeg", "jpg":
			contentType = "image/jpeg"
		case "png":
			contentType = "image/png"
		case "gif":
			contentType = "image/gif"
		case "webp":
			contentType = "image/webp"
		}
	}
	return file, contentType, nil
}
