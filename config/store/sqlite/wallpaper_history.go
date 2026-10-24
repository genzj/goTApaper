package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/genzj/goTApaper/config/store"
)

const wallpaperHistoryColumns = `id, channel_name, channel_type, url,
	COALESCE(title, ''), COALESCE(caption, ''), COALESCE(credit, ''),
	COALESCE(format, ''), COALESCE(upload_time, 0), download_time,
	COALESCE(file_path, ''), COALESCE(thumb_path, '')`

// AppendWallpaperHistory records one applied wallpaper.
//
// Unlike PutHistory there is no upsert: this table is a log, and the same
// picture applied twice is two events.
func (s *Store) AppendWallpaperHistory(e store.WallpaperHistoryEntry) (int64, error) {
	if e.ChannelName == "" {
		return 0, fmt.Errorf("a wallpaper history entry needs a channel name")
	}

	downloadTime := any(e.DownloadTime)
	if e.DownloadTime == 0 {
		// Let SQLite stamp it, so a caller that does not care about the clock
		// still gets a usable ordering.
		downloadTime = nil
	}

	res, err := s.q().Exec(
		`INSERT INTO wallpaper_history
		     (channel_name, channel_type, url, title, caption, credit, format,
		      upload_time, download_time, file_path, thumb_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE(?, unixepoch()), ?, ?)`,
		e.ChannelName, e.ChannelType, e.URL,
		nullIfEmpty(e.Title), nullIfEmpty(e.Caption), nullIfEmpty(e.Credit),
		nullIfEmpty(e.Format), nilIfZero(e.UploadTime), downloadTime,
		nullIfEmpty(e.FilePath), nullIfEmpty(e.ThumbPath),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// WallpaperHistory returns applied wallpapers newest first.
func (s *Store) WallpaperHistory(limit, offset int) ([]store.WallpaperHistoryEntry, error) {
	if offset < 0 {
		offset = 0
	}
	// SQLite needs a LIMIT before it will honour an OFFSET, and -1 is its
	// documented spelling of "no limit".
	if limit <= 0 {
		limit = -1
	}

	rows, err := s.q().Query(
		"SELECT "+wallpaperHistoryColumns+` FROM wallpaper_history
		 ORDER BY download_time DESC, id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.WallpaperHistoryEntry{}
	for rows.Next() {
		e, err := scanWallpaperHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// WallpaperHistoryCount reports the total number of recorded wallpapers.
func (s *Store) WallpaperHistoryCount() (int, error) {
	var n int
	err := s.q().QueryRow("SELECT COUNT(*) FROM wallpaper_history").Scan(&n)
	return n, err
}

// WallpaperHistoryEntryByID returns one recorded wallpaper.
func (s *Store) WallpaperHistoryEntryByID(id int64) (store.WallpaperHistoryEntry, error) {
	row := s.q().QueryRow(
		"SELECT "+wallpaperHistoryColumns+" FROM wallpaper_history WHERE id = ?", id,
	)
	e, err := scanWallpaperHistory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return store.WallpaperHistoryEntry{}, fmt.Errorf(
			"%w: wallpaper history entry %d", store.ErrNotFound, id,
		)
	}
	return e, err
}

// PruneWallpaperHistory deletes the oldest rows beyond keepAtMost, returning
// the thumbnail paths that are now unreferenced.
//
// The delete and the read of the paths share one transaction, so a concurrent
// insert cannot slip a row in between and have its thumbnail reported as
// deleted.
func (s *Store) PruneWallpaperHistory(keepAtMost int) ([]string, error) {
	if keepAtMost <= 0 {
		// Unbounded. Returning nil rather than querying keeps "no limit" from
		// costing a table scan on every refresh.
		return nil, nil
	}

	var paths []string
	err := s.Tx(func(tx store.Store) error {
		inner := tx.(*Store)

		// RETURNING rather than a select-then-delete: the two could otherwise
		// disagree about which rows the cut-off covers.
		rows, err := inner.q().Query(
			`DELETE FROM wallpaper_history
			 WHERE id IN (
			     SELECT id FROM wallpaper_history
			     ORDER BY download_time DESC, id DESC
			     LIMIT -1 OFFSET ?
			 )
			 RETURNING COALESCE(thumb_path, '')`,
			keepAtMost,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return err
			}
			if path != "" {
				paths = append(paths, path)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// WallpaperHistoryBeyond returns the thumbnail paths of the rows a prune to
// keepAtMost would delete, without deleting anything.
//
// It exists so a dry run can report what it would remove. The alternative --
// having PruneWallpaperHistory take a flag and roll back -- would mean the
// preview and the real delete shared a write path, which is a poor trade for a
// command whose whole purpose is to not write.
func (s *Store) WallpaperHistoryBeyond(keepAtMost int) ([]string, error) {
	if keepAtMost <= 0 {
		return nil, nil
	}

	rows, err := s.q().Query(
		`SELECT COALESCE(thumb_path, '') FROM wallpaper_history
		 ORDER BY download_time DESC, id DESC
		 LIMIT -1 OFFSET ?`,
		keepAtMost,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// ThumbnailPaths returns every thumbnail path a row still references.
func (s *Store) ThumbnailPaths() ([]string, error) {
	rows, err := s.q().Query(
		"SELECT thumb_path FROM wallpaper_history WHERE thumb_path IS NOT NULL AND thumb_path != ''",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		out = append(out, path)
	}
	return out, rows.Err()
}

// DeleteWallpaperHistoryEntry removes one row and returns its thumbnail path.
func (s *Store) DeleteWallpaperHistoryEntry(id int64) (string, error) {
	var path string
	err := s.q().QueryRow(
		"DELETE FROM wallpaper_history WHERE id = ? RETURNING COALESCE(thumb_path, '')",
		id,
	).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: wallpaper history entry %d", store.ErrNotFound, id)
	}
	return path, err
}

func scanWallpaperHistory(sc scanner) (store.WallpaperHistoryEntry, error) {
	var e store.WallpaperHistoryEntry
	err := sc.Scan(
		&e.ID, &e.ChannelName, &e.ChannelType, &e.URL,
		&e.Title, &e.Caption, &e.Credit, &e.Format,
		&e.UploadTime, &e.DownloadTime, &e.FilePath, &e.ThumbPath,
	)
	return e, err
}
