package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
)

// DefaultHistoryPageSize is how many entries a history page holds when the
// caller does not say.
const DefaultHistoryPageSize = 50

// MaxHistoryPageSize caps a page, so one request cannot ask for an unbounded
// response.
const MaxHistoryPageSize = 500

// HistoryPage is one page of browsable history, with the total so a client can
// render pagination without walking every row.
type HistoryPage struct {
	Entries []HistoryItem `json:"entries"`
	Total   int           `json:"total"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

// HistoryItem is a history entry as a client sees it: the stored entry, plus
// whether its thumbnail can actually be shown.
type HistoryItem struct {
	store.WallpaperHistoryEntry
	// HasThumbnail reports that the thumbnail file is on disk, not merely
	// recorded: a file can go while its entry stays, and a client should then
	// offer to load a preview rather than show a broken image.
	HasThumbnail bool `json:"hasThumbnail"`
}

func itemOf(e store.WallpaperHistoryEntry) HistoryItem {
	item := HistoryItem{WallpaperHistoryEntry: e}
	if e.ThumbPath != "" {
		if info, err := os.Stat(e.ThumbPath); err == nil && info.Size() > 0 {
			item.HasThumbnail = true
		}
	}
	return item
}

// WallpaperHistory returns one page of applied wallpapers, newest first.
func (w *Wallpaper) WallpaperHistory(limit, offset int) (HistoryPage, error) {
	if limit <= 0 {
		limit = DefaultHistoryPageSize
	}
	if limit > MaxHistoryPageSize {
		limit = MaxHistoryPageSize
	}
	if offset < 0 {
		offset = 0
	}

	entries, err := w.store.WallpaperHistory(limit, offset)
	if err != nil {
		return HistoryPage{}, err
	}
	total, err := w.store.WallpaperHistoryCount()
	if err != nil {
		return HistoryPage{}, err
	}

	items := make([]HistoryItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, itemOf(e))
	}

	return HistoryPage{
		Entries: items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// WallpaperHistoryEntry returns one recorded wallpaper.
func (w *Wallpaper) WallpaperHistoryEntry(id int64) (store.WallpaperHistoryEntry, error) {
	return w.store.WallpaperHistoryEntryByID(id)
}

// LoadPreview downloads an entry's picture again and saves its thumbnail,
// without applying it, so an entry with nothing to show can be identified
// before deciding to re-apply it. It returns the entry as updated.
//
// It does not go through the downloader: nothing on the desktop changes, so
// it need not wait for, or hold up, a download.
func (w *Wallpaper) LoadPreview(id int64) (HistoryItem, error) {
	entry, err := w.store.WallpaperHistoryEntryByID(id)
	if err != nil {
		return HistoryItem{}, err
	}
	if entry.URL == "" {
		return HistoryItem{}, fmt.Errorf("history entry %d has no source URL to preview", id)
	}

	l := logrus.WithFields(logrus.Fields{"entry": id, "url": entry.URL})
	l.Info("loading a preview for a history entry")

	_, img, _, err := util.FetchPicture(entry.URL)
	if err != nil {
		return HistoryItem{}, err
	}
	if img == nil {
		return HistoryItem{}, fmt.Errorf("no image at %s", entry.URL)
	}

	name := fmt.Sprintf("%s-%d-preview.jpg", entry.ChannelName, entry.ID)
	path, err := actor.WriteThumbnail(
		img, filepath.Join(config.GetThumbnailDir(), name), config.DefaultThumbnailWidth,
	)
	if err != nil {
		return HistoryItem{}, err
	}

	previous, err := w.store.SetThumbnail(id, path)
	if err != nil {
		_ = os.Remove(path)
		return HistoryItem{}, err
	}
	if previous != "" && previous != path {
		// Removed after the update has committed, as everywhere else.
		_ = os.Remove(previous)
	}

	entry.ThumbPath = path
	return itemOf(entry), nil
}

// Thumbnail opens the thumbnail of one history entry.
//
// It returns the open file rather than its bytes so a large image is streamed
// to the client instead of being buffered in memory first. The caller closes
// it.
func (w *Wallpaper) Thumbnail(id int64) (*os.File, error) {
	entry, err := w.store.WallpaperHistoryEntryByID(id)
	if err != nil {
		return nil, err
	}
	if entry.ThumbPath == "" {
		return nil, fmt.Errorf(
			"%w: history entry %d has no thumbnail", store.ErrNotFound, id,
		)
	}

	file, err := os.Open(entry.ThumbPath)
	if os.IsNotExist(err) {
		// The row outlived its file, which a sweep interrupted mid-way can
		// leave behind. Reported as missing rather than as a server fault.
		return nil, fmt.Errorf(
			"%w: the thumbnail for history entry %d is no longer on disk", store.ErrNotFound, id,
		)
	}
	return file, err
}

// DeleteWallpaperHistoryEntry removes one history entry and its thumbnail.
func (w *Wallpaper) DeleteWallpaperHistoryEntry(id int64) error {
	path, err := w.store.DeleteWallpaperHistoryEntry(id)
	if err != nil {
		return err
	}
	if path != "" {
		// Removed after the delete has committed, so a failure here leaves an
		// unreferenced file for the sweep rather than a row pointing at
		// nothing.
		_ = os.Remove(path)
	}
	return nil
}

// PruneHistory enforces the history entry cap and reclaims unreferenced
// thumbnails.
//
// keepAtMost of zero reads the configured cap, so a caller with no opinion
// gets the same behaviour as the automatic sweep. A negative value means
// unbounded.
func (w *Wallpaper) PruneHistory(keepAtMost int, dryRun bool) (history.SweepResult, error) {
	if keepAtMost == 0 {
		keepAtMost = config.GetInt(config.KeyHistoryMaxEntries)
	}
	if keepAtMost < 0 {
		keepAtMost = 0
	}

	return history.Sweep(w.store, history.SweepOptions{
		KeepAtMost:      keepAtMost,
		ThumbnailDir:    config.GetThumbnailDir(),
		PurgeThumbnails: !config.GetBool(config.KeyHistoryThumbnails),
		DryRun:          dryRun,
	})
}
