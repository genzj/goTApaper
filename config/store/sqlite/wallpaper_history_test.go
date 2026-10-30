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

	// The same channel applied twice is two events: this is a log, not a
	// latest-value table.
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

func TestLastAppliedURLIsTheNewestEntryOfThatChannel(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "bing", 100) // https://example.test/bing
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "bing", URL: "https://example.test/newer", DownloadTime: 200,
	}); err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}
	// Another channel's newer picture must not count: deduplication is per
	// channel instance.
	if _, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "ng", URL: "https://example.test/ng", DownloadTime: 300,
	}); err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}

	got, err := s.LastAppliedURL("bing")
	if err != nil {
		t.Fatalf("LastAppliedURL: %v", err)
	}
	if got != "https://example.test/newer" {
		t.Fatalf("LastAppliedURL(bing) = %q, want the newest bing entry", got)
	}
}

func TestLastAppliedURLWithNoHistoryIsEmpty(t *testing.T) {
	s := openMemory(t)

	// No history is the normal state of a new channel, not an error.
	got, err := s.LastAppliedURL("never-used")
	if err != nil {
		t.Fatalf("LastAppliedURL: %v", err)
	}
	if got != "" {
		t.Fatalf("LastAppliedURL = %q, want empty", got)
	}
}

func TestLastAppliedURLSurvivesPruningToOne(t *testing.T) {
	s := openMemory(t)

	appendEntry(t, s, "ng", 100)
	appendEntry(t, s, "bing", 200)

	// Even the tightest cap keeps the newest row, and that is all
	// deduplication reads for the channel that owns it.
	if _, err := s.PruneWallpaperHistory(1); err != nil {
		t.Fatalf("PruneWallpaperHistory: %v", err)
	}
	got, err := s.LastAppliedURL("bing")
	if err != nil {
		t.Fatalf("LastAppliedURL: %v", err)
	}
	if got != "https://example.test/bing" {
		t.Fatalf("LastAppliedURL = %q after pruning", got)
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
	// Applied once: first and last application are the same moment.
	want.FirstAppliedTime = want.DownloadTime
	want.TimesApplied = 1

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

func TestMarkReappliedUpdatesTheEntryInsteadOfDuplicatingIt(t *testing.T) {
	s := openMemory(t)

	old := appendEntry(t, s, "bing", 100)
	appendEntry(t, s, "ng", 200)

	if err := s.MarkReapplied(old, 300, "/app/wallpaper.jpeg", ""); err != nil {
		t.Fatalf("MarkReapplied: %v", err)
	}

	// Still two entries: the picture was applied again, not added again.
	count, _ := s.WallpaperHistoryCount()
	if count != 2 {
		t.Fatalf("WallpaperHistoryCount() = %d, want 2", count)
	}

	got, err := s.WallpaperHistoryEntryByID(old)
	if err != nil {
		t.Fatalf("WallpaperHistoryEntryByID: %v", err)
	}
	if got.FirstAppliedTime != 100 || got.DownloadTime != 300 {
		t.Errorf("first/last applied = %d/%d, want 100/300",
			got.FirstAppliedTime, got.DownloadTime)
	}
	if got.TimesApplied != 2 {
		t.Errorf("TimesApplied = %d, want 2", got.TimesApplied)
	}
	if got.FilePath != "/app/wallpaper.jpeg" {
		t.Errorf("FilePath = %q", got.FilePath)
	}
	// An empty thumbPath keeps the existing thumbnail: it is the same picture.
	if got.ThumbPath != "/thumbs/bing.jpg" {
		t.Errorf("ThumbPath = %q, want the original kept", got.ThumbPath)
	}

	// It is now the newest entry, so the history lists it first and pruning
	// keeps it as recently used.
	list, _ := s.WallpaperHistory(0, 0)
	if list[0].ID != old {
		t.Errorf("newest entry = %d, want the re-applied %d", list[0].ID, old)
	}
	url, _ := s.LastAppliedURL("bing")
	if url != "https://example.test/bing" {
		t.Errorf("LastAppliedURL = %q", url)
	}
}

func TestMarkReappliedKeepsTheFirstTimeAcrossReapplications(t *testing.T) {
	s := openMemory(t)
	id := appendEntry(t, s, "bing", 100)

	for _, at := range []int64{200, 300} {
		if err := s.MarkReapplied(id, at, "", ""); err != nil {
			t.Fatalf("MarkReapplied(%d): %v", at, err)
		}
	}

	got, _ := s.WallpaperHistoryEntryByID(id)
	if got.FirstAppliedTime != 100 || got.DownloadTime != 300 || got.TimesApplied != 3 {
		t.Fatalf("first/last/times = %d/%d/%d, want 100/300/3",
			got.FirstAppliedTime, got.DownloadTime, got.TimesApplied)
	}
}

func TestMarkReappliedCanReplaceAMissingThumbnail(t *testing.T) {
	s := openMemory(t)
	id := appendEntry(t, s, "bing", 100)

	if err := s.MarkReapplied(id, 200, "", "/thumbs/new.jpg"); err != nil {
		t.Fatalf("MarkReapplied: %v", err)
	}
	got, _ := s.WallpaperHistoryEntryByID(id)
	if got.ThumbPath != "/thumbs/new.jpg" {
		t.Fatalf("ThumbPath = %q, want the replacement", got.ThumbPath)
	}
}

func TestMarkReappliedReportsADeletedEntry(t *testing.T) {
	s := openMemory(t)

	// The entry can go while it is being re-applied; the caller then records
	// the picture afresh, so this has to be distinguishable.
	if err := s.MarkReapplied(404, 200, "", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("MarkReapplied(missing) = %v, want ErrNotFound", err)
	}
}
