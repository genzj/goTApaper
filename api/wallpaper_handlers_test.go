package api

import (
	"encoding/json"
	"fmt"
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
			URL:          fmt.Sprintf("https://example.test/pic-%d.jpg", i),
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

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReapplyRouteStartsInTheBackground(t *testing.T) {
	handlers, s := newTestHandlers(t, nil)
	h := testRoutes(handlers, newTicketStore())
	ids := seedHistory(t, s, 1)

	// Accepted, not OK: the request starts the download and returns, rather
	// than holding the connection for as long as the provider takes.
	rec := post(t, h, "/api/v1/history/"+itoa(ids[0])+"/reapply", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	// How it went arrives later, in the status (and as an event).
	st := waitIdle(t, handlers.downloader)
	if st.Last == nil || st.Last.Job.Kind != service.JobReapply || st.Last.Job.EntryID != ids[0] {
		t.Fatalf("Last = %+v, want the reapply of entry %d", st.Last, ids[0])
	}
	if st.Last.Result.Channel != "bing" {
		t.Errorf("Result.Channel = %q, want bing", st.Last.Result.Channel)
	}
}

func TestReapplyRouteRejectsAMissingEntryUpFront(t *testing.T) {
	h, _ := newTestServer(t)

	// Checked before anything starts, so the caller gets an error it can act
	// on rather than a background download that fails out of sight.
	if rec := post(t, h, "/api/v1/history/9999/reapply", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDownloadsAreRefusedWhileOneIsRunning(t *testing.T) {
	gate := make(chan struct{})
	handlers, s := newTestHandlers(t, gate)
	h := testRoutes(handlers, newTicketStore())
	ids := seedHistory(t, s, 1)

	if rec := post(t, h, "/api/v1/refresh", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("first refresh: status = %d: %s", rec.Code, rec.Body.String())
	}

	// While it is in flight, neither another refresh nor a reapply may start:
	// two downloads would race to write the same wallpaper file.
	for _, path := range []string{
		"/api/v1/refresh",
		"/api/v1/history/" + itoa(ids[0]) + "/reapply",
	} {
		if rec := post(t, h, path, ""); rec.Code != http.StatusConflict {
			t.Errorf("%s while busy: status = %d, want 409", path, rec.Code)
		}
	}

	var status StatusBody
	getJSON(t, h, "/api/v1/status", &status)
	if status.Download.State != service.StateDownloading {
		t.Errorf("status while busy = %q, want downloading", status.Download.State)
	}

	close(gate)
	waitIdle(t, handlers.downloader)

	// Once it has finished, a new one is accepted again.
	if rec := post(t, h, "/api/v1/refresh", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("refresh after finishing: status = %d", rec.Code)
	}
	waitIdle(t, handlers.downloader)
}

func TestStatusRouteReportsIdleAndTheVersion(t *testing.T) {
	h, _ := newTestServer(t)

	var status StatusBody
	if rec := getJSON(t, h, "/api/v1/status", &status); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if status.Download.State != service.StateIdle {
		t.Errorf("State = %q, want idle", status.Download.State)
	}
	if status.Version != "test" {
		t.Errorf("Version = %q", status.Version)
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

func TestLastAppliedRouteReportsTheNewestEntry(t *testing.T) {
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

	var last service.LastApplied
	if rec := getJSON(t, h, "/api/v1/wallpaper/last-applied", &last); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	if last.Entry == nil || last.Entry.Title != "Current" {
		t.Fatalf("Entry = %+v", last.Entry)
	}
	if !last.Exists {
		t.Error("Exists = false for a file that is on disk")
	}
	if last.Size != int64(len("pretend-jpeg")) {
		t.Errorf("Size = %d", last.Size)
	}
}

func TestLastAppliedRouteWithNoHistory(t *testing.T) {
	h, _ := newTestServer(t)

	var last service.LastApplied
	rec := getJSON(t, h, "/api/v1/wallpaper/last-applied", &last)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with an empty body", rec.Code)
	}
	if last.Entry != nil {
		t.Errorf("Entry = %+v, want nil", last.Entry)
	}
	if last.Exists {
		t.Error("Exists = true with no wallpaper applied")
	}

	// The image route has nothing to serve, which is a 404 rather than a 200
	// with an empty body.
	if rec := getJSON(t, h, "/api/v1/wallpaper/last-applied/image", nil); rec.Code != http.StatusNotFound {
		t.Errorf("image: status = %d, want 404", rec.Code)
	}
}

func TestLastAppliedImageIsNotCached(t *testing.T) {
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

	rec := getJSON(t, h, "/api/v1/wallpaper/last-applied/image", nil)
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
