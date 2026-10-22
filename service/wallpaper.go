package service

import (
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config/store"
)

// RefreshFunc downloads and applies a wallpaper. It is supplied by the caller
// rather than implemented here, because the pipeline lives in cmd alongside
// the cobra commands that also drive it; injecting it keeps one
// implementation rather than two.
type RefreshFunc func(channels []string) (*channel.PictureMeta, error)

// Wallpaper exposes wallpaper operations.
type Wallpaper struct {
	store   store.Store
	refresh RefreshFunc
}

// NewWallpaper builds a wallpaper service.
func NewWallpaper(s store.Store, refresh RefreshFunc) *Wallpaper {
	return &Wallpaper{store: s, refresh: refresh}
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

// History returns the most recent download recorded for each channel.
func (w *Wallpaper) History() ([]store.HistoryEntry, error) {
	return w.store.Histories()
}
