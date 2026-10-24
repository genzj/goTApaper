package service

import (
	"errors"
	"fmt"
	"image"

	// Registered for their DecodeConfig, so reporting a wallpaper's
	// dimensions works for whatever format a channel served.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config/store"
)

// RefreshFunc downloads and applies a wallpaper. It is supplied by the caller
// rather than implemented here, because the pipeline lives in cmd alongside
// the cobra commands that also drive it; injecting it keeps one
// implementation rather than two.
type RefreshFunc func(channels []string) (*channel.PictureMeta, error)

// ReapplyFunc downloads a picture from a URL and applies it as the wallpaper,
// as if a channel had just served it. Supplied by the caller for the same
// reason RefreshFunc is: the pipeline lives in cmd.
type ReapplyFunc func(entry store.WallpaperHistoryEntry) (*channel.PictureMeta, error)

// Wallpaper exposes wallpaper operations.
type Wallpaper struct {
	store   store.Store
	refresh RefreshFunc
	reapply ReapplyFunc
}

// NewWallpaper builds a wallpaper service.
//
// reapply may be nil, in which case re-applying a history entry is reported as
// unsupported rather than panicking. That keeps the service constructible from
// a test that only exercises configuration.
func NewWallpaper(s store.Store, refresh RefreshFunc, reapply ReapplyFunc) *Wallpaper {
	return &Wallpaper{store: s, refresh: refresh, reapply: reapply}
}

// RefreshResult reports the outcome of a refresh.
type RefreshResult struct {
	// Channel is the channel key that supplied the picture.
	Channel string `json:"channel"`
	// Type is the channel implementation used.
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	// Credit names the photographer or uploader.
	Credit string `json:"credit,omitempty"`
}

// RefreshNow downloads and applies a wallpaper immediately. An empty channels
// slice uses the configured rotation; naming channels runs those instead, even
// if they are disabled.
func (w *Wallpaper) RefreshNow(channels []string) (RefreshResult, error) {
	meta, err := w.refresh(channels)
	if err != nil {
		return RefreshResult{}, err
	}
	if meta == nil {
		return RefreshResult{}, nil
	}
	return RefreshResult{
		Channel: meta.ChannelKey,
		Type:    meta.Channel,
		Title:   meta.Title,
		Credit:  meta.Credit,
	}, nil
}

// History returns the most recent download recorded for each channel. This is
// the deduplication pointer, one row per channel; WallpaperHistory is the
// browsable log.
func (w *Wallpaper) History() ([]store.HistoryEntry, error) {
	return w.store.Histories()
}

// Reapply downloads a recorded wallpaper again and sets it.
//
// The stored URL may have expired -- several providers serve time-limited
// links -- in which case the download fails and the current wallpaper is left
// alone.
func (w *Wallpaper) Reapply(id int64) (RefreshResult, error) {
	entry, err := w.store.WallpaperHistoryEntryByID(id)
	if err != nil {
		return RefreshResult{}, err
	}
	if entry.URL == "" {
		return RefreshResult{}, fmt.Errorf(
			"history entry %d has no source URL to download again", id,
		)
	}
	if w.reapply == nil {
		return RefreshResult{}, errors.New("re-applying a wallpaper is not available here")
	}

	meta, err := w.reapply(entry)
	if err != nil {
		return RefreshResult{}, err
	}
	if meta == nil {
		return RefreshResult{}, nil
	}
	return RefreshResult{
		Channel: meta.ChannelKey,
		Type:    meta.Channel,
		Title:   meta.Title,
		Credit:  meta.Credit,
	}, nil
}

// CurrentWallpaper describes the wallpaper now on the desktop: the newest
// history entry, plus what is actually on disk.
type CurrentWallpaper struct {
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

// Current returns the wallpaper now applied.
func (w *Wallpaper) Current() (CurrentWallpaper, error) {
	out := CurrentWallpaper{}

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

// CurrentImage opens the wallpaper file now applied, for streaming to a
// client. The caller closes it.
func (w *Wallpaper) CurrentImage() (*os.File, string, error) {
	current, err := w.Current()
	if err != nil {
		return nil, "", err
	}
	if current.Path == "" {
		return nil, "", fmt.Errorf("%w: no wallpaper has been applied yet", store.ErrNotFound)
	}

	file, err := os.Open(current.Path)
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf(
			"%w: the wallpaper file %s is no longer on disk", store.ErrNotFound, current.Path,
		)
	}
	if err != nil {
		return nil, "", err
	}

	contentType := "application/octet-stream"
	if current.Entry != nil {
		switch current.Entry.Format {
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
