package sqlite

import (
	"errors"
	"slices"
	"testing"

	"github.com/genzj/goTApaper/config/store"
)

// appendEntry records one wallpaper, with a download time supplied so the
// ordering under test is deterministic rather than at the mercy of whether two
// inserts land in the same second.
func appendEntry(t *testing.T, s *Store, channelName string, at int64) int64 {
	t.Helper()

	id, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName:  channelName,
		ChannelType:  "bing-wallpaper",
		URL:          "https://example.test/" + channelName,
		Title:        "a title",
		DownloadTime: at,
		ThumbPath:    "/thumbs/" + channelName + ".jpg",
	})
	if err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}
	return id
}

func TestAppendWallpaperHistoryIsALogNotAnUpsert(t *testing.T) {
	s := openMemory(t)

	// The same channel applied twice is two events. This is the whole reason
	// the table exists separately from download_history, whose primary key on
	// channel_name makes the second write replace the first.
	appendEntry(t, s, "bing", 100)
	appendEntry(t, s, "bing", 200)

	count, err := s.WallpaperHistoryCount()
	if err != nil {
		t.Fatalf("WallpaperHistoryCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("WallpaperHistoryCount() = %d, want 2", count)
	}
}

func TestAppendWallpaperHistoryDoesNotDisturbTheDedupPointer(t *testing.T) {
	s := openMemory(t)

	if err := s.PutHistory(store.HistoryEntry{
		ChannelName: "bing", URL: "https://example.test/first",
	}); err != nil {
		t.Fatalf("PutHistory: %v", err)
	}
	appendEntry(t, s, "bing", 100)

	// Deduplication must still see exactly what it stored: the browsable
	// history is a separate concern and changing it would change which
	// pictures get re-downloaded.
	got, err := s.History("bing")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if got.URL != "https://example.test/first" {
		t.Fatalf("dedup URL = %q, want the one PutHistory stored", got.URL)
	}
}

func TestAppendWallpaperHistoryRoundTripsEveryField(t *testing.T) {
	s := openMemory(t)

	want := store.WallpaperHistoryEntry{
		ChannelName:  "unsplash-kw",
		ChannelType:  "unsplash",
		URL:          "https://example.test/photo.jpg",
		Title:        "Mountains",
		Caption:      "Taken at dawn",
		Credit:       "A Photographer",
		Format:       "jpeg",
		UploadTime:   1600000000,
		DownloadTime: 1700000000,
		FilePath:     "/app/wallpaper.jpeg",
		ThumbPath:    "/app/thumbs/unsplash-kw-1700000000.jpg",
	}

	id, err := s.AppendWallpaperHistory(want)
	if err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}
	want.ID = id

	got, err := s.WallpaperHistoryEntryByID(id)
	if err != nil {
		t.Fatalf("WallpaperHistoryEntryByID: %v", err)
	}
	if got != want {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestAppendWallpaperHistoryStampsAMissingDownloadTime(t *testing.T) {
	s := openMemory(t)

	id, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{ChannelName: "bing"})
	if err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}

	got, err := s.WallpaperHistoryEntryByID(id)
	if err != nil {
		t.Fatalf("WallpaperHistoryEntryByID: %v", err)
	}
	if got.DownloadTime == 0 {
		t.Fatal("DownloadTime was not stamped")
	}
	// An absent upload time must stay distinguishable from 1970.
	if got.UploadTime != 0 {
		t.Fatalf("UploadTime = %d, want 0", got.UploadTime)
	}
}

func TestAppendWallpaperHistoryRequiresAChannelName(t *testing.T) {
	s := openMemory(t)

	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{}); err == nil {
		t.Fatal("AppendWallpaperHistory accepted an entry with no channel name")
	}
}

func TestWallpaperHistoryIsNewestFirstAndPages(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "oldest", 100)
	appendEntry(t, s, "middle", 200)
	appendEntry(t, s, "newest", 300)

	all, err := s.WallpaperHistory(0, 0)
	if err != nil {
		t.Fatalf("WallpaperHistory: %v", err)
	}
	if got := names(all); !slices.Equal(got, []string{"newest", "middle", "oldest"}) {
		t.Fatalf("WallpaperHistory() = %v, want newest first", got)
	}

	// An offset without a limit must still work: SQLite ignores OFFSET unless
	// a LIMIT is present, so "page 2 of everything" is a real case to cover.
	rest, err := s.WallpaperHistory(0, 1)
	if err != nil {
		t.Fatalf("WallpaperHistory(0, 1): %v", err)
	}
	if got := names(rest); !slices.Equal(got, []string{"middle", "oldest"}) {
		t.Fatalf("WallpaperHistory(0, 1) = %v, want the last two", got)
	}

	page, err := s.WallpaperHistory(1, 1)
	if err != nil {
		t.Fatalf("WallpaperHistory(1, 1): %v", err)
	}
	if got := names(page); !slices.Equal(got, []string{"middle"}) {
		t.Fatalf("WallpaperHistory(1, 1) = %v, want [middle]", got)
	}
}

func TestWallpaperHistoryEntryByIDReportsNotFound(t *testing.T) {
	s := openMemory(t)

	_, err := s.WallpaperHistoryEntryByID(404)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("WallpaperHistoryEntryByID(404) = %v, want ErrNotFound", err)
	}
}

func TestPruneWallpaperHistoryKeepsTheNewestAndReportsTheThumbnails(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "oldest", 100)
	appendEntry(t, s, "middle", 200)
	appendEntry(t, s, "newest", 300)

	paths, err := s.PruneWallpaperHistory(1)
	if err != nil {
		t.Fatalf("PruneWallpaperHistory: %v", err)
	}

	// The paths of the deleted rows are returned so the caller can unlink the
	// files after the transaction commits.
	slices.Sort(paths)
	want := []string{"/thumbs/middle.jpg", "/thumbs/oldest.jpg"}
	if !slices.Equal(paths, want) {
		t.Fatalf("PruneWallpaperHistory(1) = %v, want %v", paths, want)
	}

	left, err := s.WallpaperHistory(0, 0)
	if err != nil {
		t.Fatalf("WallpaperHistory: %v", err)
	}
	if got := names(left); !slices.Equal(got, []string{"newest"}) {
		t.Fatalf("remaining = %v, want [newest]", got)
	}
}

func TestPruneWallpaperHistoryWithNoCapKeepsEverything(t *testing.T) {
	s := openMemory(t)
	appendEntry(t, s, "bing", 100)

	for _, keep := range []int{0, -1} {
		paths, err := s.PruneWallpaperHistory(keep)
		if err != nil {
			t.Fatalf("PruneWallpaperHistory(%d): %v", keep, err)
		}
		if len(paths) != 0 {
			t.Fatalf("PruneWallpaperHistory(%d) deleted %v, want nothing", keep, paths)
		}
	}

	count, _ := s.WallpaperHistoryCount()
	if count != 1 {
		t.Fatalf("WallpaperHistoryCount() = %d, want 1", count)
	}
}

func TestPruneWallpaperHistoryUnderTheCapDeletesNothing(t *testing.T) {
	s := openMemory(t)
	appendEntry(t, s, "bing", 100)

	paths, err := s.PruneWallpaperHistory(10)
	if err != nil {
		t.Fatalf("PruneWallpaperHistory: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("deleted %v, want nothing", paths)
	}
}

func TestThumbnailPathsReturnsOnlyReferencedPaths(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "with-thumb", 100)
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "without-thumb", DownloadTime: 200,
	}); err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}

	paths, err := s.ThumbnailPaths()
	if err != nil {
		t.Fatalf("ThumbnailPaths: %v", err)
	}
	// A row with no thumbnail must not contribute an empty string, or the
	// orphan sweep would treat "" as a referenced name.
	if !slices.Equal(paths, []string{"/thumbs/with-thumb.jpg"}) {
		t.Fatalf("ThumbnailPaths() = %v", paths)
	}
}

func TestDeleteWallpaperHistoryEntryReturnsItsThumbnail(t *testing.T) {
	s := openMemory(t)
	id := appendEntry(t, s, "bing", 100)

	path, err := s.DeleteWallpaperHistoryEntry(id)
	if err != nil {
		t.Fatalf("DeleteWallpaperHistoryEntry: %v", err)
	}
	if path != "/thumbs/bing.jpg" {
		t.Fatalf("returned path = %q", path)
	}

	if _, err := s.WallpaperHistoryEntryByID(id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the entry survived the delete: %v", err)
	}
	if _, err := s.DeleteWallpaperHistoryEntry(id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleting it twice = %v, want ErrNotFound", err)
	}
}

func TestDeleteChannelAlsoDeletesItsBrowsableHistory(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "bing", 100)
	appendEntry(t, s, "ng", 200)

	// "bing" is a seeded channel, so this exercises the real path.
	if err := s.DeleteChannel("bing"); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}

	left, err := s.WallpaperHistory(0, 0)
	if err != nil {
		t.Fatalf("WallpaperHistory: %v", err)
	}
	if got := names(left); !slices.Equal(got, []string{"ng"}) {
		t.Fatalf("remaining history = %v, want only ng's", got)
	}
}

func names(entries []store.WallpaperHistoryEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ChannelName)
	}
	return out
}

func TestWallpaperHistoryBeyondPreviewsWithoutDeleting(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "oldest", 100)
	appendEntry(t, s, "newest", 200)

	paths, err := s.WallpaperHistoryBeyond(1)
	if err != nil {
		t.Fatalf("WallpaperHistoryBeyond: %v", err)
	}
	// Exactly the set a prune to the same cap would delete.
	if !slices.Equal(paths, []string{"/thumbs/oldest.jpg"}) {
		t.Fatalf("WallpaperHistoryBeyond(1) = %v", paths)
	}

	count, _ := s.WallpaperHistoryCount()
	if count != 2 {
		t.Fatalf("the preview deleted rows: %d left, want 2", count)
	}
}

func TestWallpaperHistoryBeyondAgreesWithPrune(t *testing.T) {
	s := openMemory(t)
	for i := range 5 {
		appendEntry(t, s, "ch", int64(100+i))
	}

	preview, err := s.WallpaperHistoryBeyond(2)
	if err != nil {
		t.Fatalf("WallpaperHistoryBeyond: %v", err)
	}
	deleted, err := s.PruneWallpaperHistory(2)
	if err != nil {
		t.Fatalf("PruneWallpaperHistory: %v", err)
	}

	slices.Sort(preview)
	slices.Sort(deleted)
	if !slices.Equal(preview, deleted) {
		t.Fatalf("preview %v does not match the delete %v", preview, deleted)
	}
}

func TestWallpaperHistoryBeyondWithNoCapPreviewsNothing(t *testing.T) {
	s := openMemory(t)
	appendEntry(t, s, "ch", 100)

	paths, err := s.WallpaperHistoryBeyond(0)
	if err != nil {
		t.Fatalf("WallpaperHistoryBeyond: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("WallpaperHistoryBeyond(0) = %v, want nothing", paths)
	}
}
