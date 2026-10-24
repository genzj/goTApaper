package service

import (
	"fmt"
	"os"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/history"
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
	Entries []store.WallpaperHistoryEntry `json:"entries"`
	Total   int                           `json:"total"`
	Limit   int                           `json:"limit"`
	Offset  int                           `json:"offset"`
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

	return HistoryPage{
		Entries: entries,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// WallpaperHistoryEntry returns one recorded wallpaper.
func (w *Wallpaper) WallpaperHistoryEntry(id int64) (store.WallpaperHistoryEntry, error) {
	return w.store.WallpaperHistoryEntryByID(id)
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
