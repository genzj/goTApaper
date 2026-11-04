package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/config/store/sqlite"
)

func init() {
	config.SetAppName("goTApaper")
}

// fakePipeline stands in for the download pipeline. Each call blocks until
// release is closed or receives, so a test controls exactly when a download
// finishes.
type fakePipeline struct {
	release  chan struct{}
	refreshs atomic.Int32
	reapplys atomic.Int32
	err      error
}

func (f *fakePipeline) refresh([]string, bool) (*channel.PictureMeta, error) {
	f.refreshs.Add(1)
	<-f.release
	return &channel.PictureMeta{ChannelKey: "bing", Title: "t"}, f.err
}

func (f *fakePipeline) reapply(e store.WallpaperHistoryEntry) (*channel.PictureMeta, error) {
	f.reapplys.Add(1)
	<-f.release
	return &channel.PictureMeta{ChannelKey: e.ChannelName}, f.err
}

func newTestDownloader(t *testing.T) (*Downloader, *fakePipeline, store.Store) {
	t.Helper()
	s, err := sqlite.Open(sqlite.MemoryPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	f := &fakePipeline{release: make(chan struct{})}
	return NewDownloader(s, f.refresh, f.reapply), f, s
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func idle(d *Downloader) func() bool {
	return func() bool { return d.Status().State == StateIdle }
}

func TestDownloaderRefusesAUserRequestWhileBusy(t *testing.T) {
	d, f, _ := newTestDownloader(t)

	if err := d.Refresh(nil, false, "test"); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	if err := d.Refresh(nil, false, "test"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Refresh = %v, want ErrBusy", err)
	}

	st := d.Status()
	if st.State != StateDownloading || st.Job == nil || st.Job.Kind != JobRefresh {
		t.Fatalf("Status = %+v, want a refresh in progress", st)
	}

	close(f.release)
	waitFor(t, "idle", idle(d))

	// A refused request must not have been queued behind the first: a user
	// who was told "busy" did not ask for a picture to appear later.
	time.Sleep(20 * time.Millisecond)
	if got := f.refreshs.Load(); got != 1 {
		t.Fatalf("pipeline ran %d times, want 1", got)
	}
}

func TestDownloaderRunsOneScheduledRefreshAfterTheCurrentDownload(t *testing.T) {
	d, f, _ := newTestDownloader(t)

	if err := d.Refresh(nil, false, "test"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	// Three scheduled triggers while busy -- a config change and two timer
	// ticks, say -- all want the same thing, so they coalesce into one.
	d.Schedule("test")
	d.Schedule("test")
	d.Schedule("test")

	f.release <- struct{}{} // finish the user's refresh
	waitFor(t, "the scheduled refresh to start", func() bool {
		return f.refreshs.Load() == 2
	})
	if st := d.Status(); st.Job == nil || !st.Job.Scheduled {
		t.Fatalf("Status = %+v, want the scheduled refresh running", st)
	}

	close(f.release)
	waitFor(t, "idle", idle(d))
	time.Sleep(20 * time.Millisecond)
	if got := f.refreshs.Load(); got != 2 {
		t.Fatalf("pipeline ran %d times, want 2", got)
	}
}

func TestDownloaderReapplyChecksTheEntryBeforeStarting(t *testing.T) {
	d, f, s := newTestDownloader(t)

	if err := d.Reapply(404, "test"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Reapply(missing) = %v, want ErrNotFound", err)
	}

	id, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{ChannelName: "bing"})
	if err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}
	if err := d.Reapply(id, "test"); err == nil {
		t.Fatal("Reapply accepted an entry with no URL to download")
	}

	if got := f.reapplys.Load(); got != 0 {
		t.Fatalf("pipeline ran %d times for invalid requests, want 0", got)
	}
	if st := d.Status(); st.State != StateIdle {
		t.Fatalf("an invalid request left the downloader %q", st.State)
	}
}

func TestDownloaderRecordsTheOutcomeAndNotifies(t *testing.T) {
	d, f, s := newTestDownloader(t)
	f.err = errors.New("the link expired")

	id, err := s.AppendWallpaperHistory(store.WallpaperHistoryEntry{
		ChannelName: "ng", URL: "https://example.invalid/a.jpg",
	})
	if err != nil {
		t.Fatalf("AppendWallpaperHistory: %v", err)
	}

	var mu sync.Mutex
	var events []DownloadEvent
	d.OnChange(func(e DownloadEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	if err := d.Reapply(id, "test"); err != nil {
		t.Fatalf("Reapply: %v", err)
	}
	close(f.release)
	waitFor(t, "idle", idle(d))

	st := d.Status()
	if st.Last == nil || st.Last.Job.EntryID != id || st.Last.Error != "the link expired" {
		t.Fatalf("Last = %+v, want the failed reapply of %d", st.Last, id)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0].Finished || !events[1].Finished {
		t.Fatalf("events = %+v, want started then finished", events)
	}
	if events[1].Err == nil {
		t.Error("the finished event does not carry the error")
	}
}
