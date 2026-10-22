package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/genzj/goTApaper/config/store"
)

const historyColumns = `channel_name, url, COALESCE(title, ''), COALESCE(credit, ''),
	COALESCE(upload_time, 0), download_time, COALESCE(file_path, '')`

// History returns the recorded download for one channel.
//
// There is at most one row per channel: this mirrors the history.json file it
// replaces, where History was a map[string]History holding a single URL, and
// Has was a plain equality check.
func (s *Store) History(channelName string) (store.HistoryEntry, error) {
	row := s.q().QueryRow(
		"SELECT "+historyColumns+" FROM download_history WHERE channel_name = ?",
		channelName,
	)
	e, err := scanHistory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return store.HistoryEntry{}, fmt.Errorf(
			"%w: history for channel %q", store.ErrNotFound, channelName,
		)
	}
	return e, err
}

// Histories returns every channel's recorded download, most recent first.
func (s *Store) Histories() ([]store.HistoryEntry, error) {
	rows, err := s.q().Query(
		"SELECT " + historyColumns + " FROM download_history ORDER BY download_time DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.HistoryEntry
	for rows.Next() {
		e, err := scanHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanHistory(sc scanner) (store.HistoryEntry, error) {
	var e store.HistoryEntry
	err := sc.Scan(
		&e.ChannelName, &e.URL, &e.Title, &e.Credit,
		&e.UploadTime, &e.DownloadTime, &e.FilePath,
	)
	return e, err
}

// PutHistory records a channel's latest download, replacing any previous
// entry for that channel.
func (s *Store) PutHistory(e store.HistoryEntry) error {
	if e.ChannelName == "" {
		return fmt.Errorf("history entry needs a channel name")
	}

	downloadTime := any(e.DownloadTime)
	if e.DownloadTime == 0 {
		// Let SQLite stamp it, so a caller that does not care about the clock
		// still gets a usable ordering.
		downloadTime = nil
	}

	_, err := s.q().Exec(
		`INSERT INTO download_history
		     (channel_name, url, title, credit, upload_time, download_time, file_path)
		 VALUES (?, ?, ?, ?, ?, COALESCE(?, unixepoch()), ?)
		 ON CONFLICT(channel_name) DO UPDATE SET
		     url = excluded.url,
		     title = excluded.title,
		     credit = excluded.credit,
		     upload_time = excluded.upload_time,
		     download_time = excluded.download_time,
		     file_path = excluded.file_path`,
		e.ChannelName, e.URL, nullIfEmpty(e.Title), nullIfEmpty(e.Credit),
		nilIfZero(e.UploadTime), downloadTime, nullIfEmpty(e.FilePath),
	)
	return err
}

// nilIfZero maps an absent timestamp to SQL NULL rather than to the Unix
// epoch, so "unknown upload time" stays distinguishable from 1970.
func nilIfZero(ts int64) any {
	if ts == 0 {
		return nil
	}
	return ts
}
