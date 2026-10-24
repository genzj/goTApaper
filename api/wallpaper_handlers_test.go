package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/service"
)

// seedHistory records entries so the history routes have something to serve.
// Thumbnail files are written for real, because several of these routes exist
// to stream them.
func seedHistory(t *testing.T, s store.Store, count int) []int64 {
	t.Helper()

	dir := t.TempDir()
	var ids []int64
	for i := range count {
		thumb := filepath.Join(dir, "thumb-"+string(rune('a'+i))+".jpg")
		if err := os.WriteFile(thumb, []byte("not-really-a-jpeg"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		id, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
			ChannelName:  "bing",
			ChannelType:  "bing-wallpaper",
			URL:          "https://example.test/pic.jpg",
			Title:        "Picture",
			Format:       "jpeg",
			DownloadTime: int64(1000 + i),
			ThumbPath:    thumb,
		})
		if err != nil {
			t.Fatalf("AppendWallpaperHistory: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func getJSON(t *testing.T, h http.Handler, path string, out any) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if out != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s: cannot decode %q: %v", path, rec.Body.String(), err)
		}
	}
	return rec
}

func TestHistoryRouteServesTheBrowsableLog(t *testing.T) {
	h, s := newTestServer(t)
	seedHistory(t, s, 3)

	var page service.HistoryPage
	rec := getJSON(t, h, "/api/v1/history", &page)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	if page.Total != 3 {
		t.Errorf("Total = %d, want 3", page.Total)
	}
	if len(page.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(page.Entries))
	}
	// Newest first.
	if page.Entries[0].DownloadTime < page.Entries[2].DownloadTime {
		t.Error("entries are not newest first")
	}
}

func TestHistoryRoutePages(t *testing.T) {
	h, s := newTestServer(t)
	seedHistory(t, s, 5)

	var page service.HistoryPage
	getJSON(t, h, "/api/v1/history?limit=2&offset=1", &page)

	if len(page.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(page.Entries))
	}
	if page.Total != 5 {
		t.Errorf("Total = %d, want 5", page.Total)
	}
	if page.Limit != 2 || page.Offset != 1 {
		t.Errorf("Limit/Offset = %d/%d, want 2/1", page.Limit, page.Offset)
	}
}

func TestHistoryRouteCapsThePageSize(t *testing.T) {
	h, s := newTestServer(t)
	seedHistory(t, s, 1)

	var page service.HistoryPage
	getJSON(t, h, "/api/v1/history?limit=99999", &page)

	// One request must not be able to ask for an unbounded response.
	if page.Limit != service.MaxHistoryPageSize {
		t.Fatalf("Limit = %d, want %d", page.Limit, service.MaxHistoryPageSize)
	}
}

func TestLatestPerChannelRouteStillServesTheDedupPointer(t *testing.T) {
	h, s := newTestServer(t)
	if err := s.PutHistory(store.HistoryEntry{
		ChannelName: "bing", URL: "https://example.test/dedup.jpg",
	}); err != nil {
		t.Fatalf("PutHistory: %v", err)
	}

	// Moving the browsable history onto /history must not have taken the
	// deduplication view away from the CLI.
	var entries []store.HistoryEntry
	rec := getJSON(t, h, "/api/v1/history/latest-per-channel", &entries)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(entries) != 1 || entries[0].URL != "https://example.test/dedup.jpg" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestHistoryEntryRouteAndNotFound(t *testing.T) {
	h, s := newTestServer(t)
	ids := seedHistory(t, s, 1)

	var entry store.WallpaperHistoryEntry
	if rec := getJSON(t, h, "/api/v1/history/"+itoa(ids[0]), &entry); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if entry.ID != ids[0] {
		t.Errorf("ID = %d, want %d", entry.ID, ids[0])
	}

	if rec := getJSON(t, h, "/api/v1/history/9999", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing entry: status = %d, want 404", rec.Code)
	}
	if rec := getJSON(t, h, "/api/v1/history/not-a-number", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: status = %d, want 400", rec.Code)
	}
}

func TestThumbnailRouteStreamsTheFile(t *testing.T) {
	h, s := newTestServer(t)
	ids := seedHistory(t, s, 1)

	rec := getJSON(t, h, "/api/v1/history/"+itoa(ids[0])+"/thumbnail", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q", got)
	}
	if rec.Body.String() != "not-really-a-jpeg" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestThumbnailRouteReportsAMissingFileAsNotFound(t *testing.T) {
	h, s := newTestServer(t)
	ids := seedHistory(t, s, 1)

	entry, err := s.WallpaperHistoryEntryByID(ids[0])
	if err != nil {
		t.Fatalf("WallpaperHistoryEntryByID: %v", err)
	}
	// A sweep interrupted midway can leave a row whose file has gone. That is
	// missing data, not a server fault.
	if err := os.Remove(entry.ThumbPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if rec := getJSON(t, h, "/api/v1/history/"+itoa(ids[0])+"/thumbnail", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteHistoryEntryRemovesTheRowAndItsThumbnail(t *testing.T) {
	h, s := newTestServer(t)
	ids := seedHistory(t, s, 1)

	entry, _ := s.WallpaperHistoryEntryByID(ids[0])

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/history/"+itoa(ids[0]), nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(entry.ThumbPath); !os.IsNotExist(err) {
		t.Error("the thumbnail file survived the delete")
	}
}

func TestReapplyRouteAppliesARecordedEntry(t *testing.T) {
	h, s := newTestServer(t)
	ids := seedHistory(t, s, 1)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/history/"+itoa(ids[0])+"/reapply", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var result service.RefreshResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("cannot decode: %v", err)
	}
	if result.Channel != "bing" {
		t.Errorf("Channel = %q, want bing", result.Channel)
	}
}

func TestPruneRouteReportsWhatItRemoved(t *testing.T) {
	h, s := newTestServer(t)
	seedHistory(t, s, 4)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/history/prune",
		readerOf(`{"keepAtMost":1}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		RowsRemoved  int  `json:"rowsRemoved"`
		FilesRemoved int  `json:"filesRemoved"`
		DryRun       bool `json:"dryRun"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("cannot decode: %v", err)
	}
	if result.RowsRemoved != 3 {
		t.Errorf("RowsRemoved = %d, want 3", result.RowsRemoved)
	}

	count, _ := s.WallpaperHistoryCount()
	if count != 1 {
		t.Errorf("%d rows left, want 1", count)
	}
}

func TestCurrentWallpaperRouteReportsTheNewestEntry(t *testing.T) {
	h, s := newTestServer(t)

	dir := t.TempDir()
	wallpaper := filepath.Join(dir, "wallpaper.jpeg")
	if err := os.WriteFile(wallpaper, []byte("pretend-jpeg"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "bing", Title: "Current", Format: "jpeg",
		DownloadTime: 2000, FilePath: wallpaper,
	}); err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}

	var current service.CurrentWallpaper
	if rec := getJSON(t, h, "/api/v1/wallpaper/current", &current); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	if current.Entry == nil || current.Entry.Title != "Current" {
		t.Fatalf("Entry = %+v", current.Entry)
	}
	if !current.Exists {
		t.Error("Exists = false for a file that is on disk")
	}
	if current.Size != int64(len("pretend-jpeg")) {
		t.Errorf("Size = %d", current.Size)
	}
}

func TestCurrentWallpaperRouteWithNoHistory(t *testing.T) {
	h, _ := newTestServer(t)

	var current service.CurrentWallpaper
	rec := getJSON(t, h, "/api/v1/wallpaper/current", &current)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with an empty body", rec.Code)
	}
	if current.Entry != nil {
		t.Errorf("Entry = %+v, want nil", current.Entry)
	}
	if current.Exists {
		t.Error("Exists = true with no wallpaper applied")
	}

	// The image route has nothing to serve, which is a 404 rather than a 200
	// with an empty body.
	if rec := getJSON(t, h, "/api/v1/wallpaper/current/image", nil); rec.Code != http.StatusNotFound {
		t.Errorf("image: status = %d, want 404", rec.Code)
	}
}

func TestCurrentWallpaperImageIsNotCached(t *testing.T) {
	h, s := newTestServer(t)

	wallpaper := filepath.Join(t.TempDir(), "wallpaper.jpeg")
	if err := os.WriteFile(wallpaper, []byte("pretend-jpeg"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "bing", Format: "jpeg", DownloadTime: 2000, FilePath: wallpaper,
	}); err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}

	rec := getJSON(t, h, "/api/v1/wallpaper/current/image", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
	// This path keeps serving a different picture as refreshes replace the
	// file underneath it, so caching it would show a stale wallpaper.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// itoa and readerOf keep the table-ish tests above readable.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func readerOf(body string) io.Reader { return strings.NewReader(body) }
