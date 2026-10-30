package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config/store"
)

// ErrBusy is returned when a download is asked for while another is running.
//
// Only one download runs at a time. Two concurrent ones would race to write
// the same wallpaper file and call the setter twice, and whichever finished
// last would win -- not necessarily the one the user asked for most recently.
var ErrBusy = errors.New("a wallpaper download is already in progress")

// RefreshFunc downloads and applies a wallpaper. It is supplied by the caller
// rather than implemented here, because the pipeline lives in cmd alongside
// the cobra commands that also drive it; injecting it keeps one
// implementation rather than two.
//
// force downloads even a picture the channel applied last time.
type RefreshFunc func(channels []string, force bool) (*channel.PictureMeta, error)

// ReapplyFunc downloads a picture from a URL and applies it as the wallpaper,
// as if a channel had just served it. Supplied by the caller for the same
// reason RefreshFunc is: the pipeline lives in cmd.
type ReapplyFunc func(entry store.WallpaperHistoryEntry) (*channel.PictureMeta, error)

// DownloadState is what the downloader is doing.
type DownloadState string

// Download states.
const (
	StateIdle        DownloadState = "idle"
	StateDownloading DownloadState = "downloading"
)

// JobKind names what a download is for.
type JobKind string

// Job kinds.
const (
	// JobRefresh asks the channels for a new picture.
	JobRefresh JobKind = "refresh"
	// JobReapply downloads a recorded picture again.
	JobReapply JobKind = "reapply"
)

// Job describes one download.
type Job struct {
	Kind JobKind `json:"kind"`
	// Channels names the channels a refresh runs, empty meaning the
	// configured rotation.
	Channels []string `json:"channels,omitempty"`
	// Force downloads even a picture the channel applied last time.
	Force bool `json:"force,omitempty"`
	// EntryID is the history entry a reapply downloads again.
	EntryID int64 `json:"entryId,omitempty"`
	// Scheduled is set for a refresh the timer or a configuration change
	// started, rather than a user.
	Scheduled bool `json:"scheduled,omitempty"`
}

// RefreshResult reports the picture a download applied.
type RefreshResult struct {
	// Channel is the channel key that supplied the picture.
	Channel string `json:"channel"`
	// Type is the channel implementation used.
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	// Credit names the photographer or uploader.
	Credit string `json:"credit,omitempty"`
}

// Outcome is how a finished download went.
type Outcome struct {
	Job        Job           `json:"job"`
	Result     RefreshResult `json:"result"`
	Error      string        `json:"error,omitempty"`
	FinishedAt int64         `json:"finishedAt"`
}

// DownloadStatus is the downloader's state, for a client deciding whether to
// offer a download at all.
type DownloadStatus struct {
	State DownloadState `json:"state"`
	// Job and StartedAt describe the download in progress.
	Job       *Job  `json:"job,omitempty"`
	StartedAt int64 `json:"startedAt,omitempty"`
	// Last is the most recent finished download.
	Last *Outcome `json:"last,omitempty"`
}

// DownloadEvent reports a download starting or finishing.
type DownloadEvent struct {
	Job Job
	// Finished is false when the download starts and true when it ends.
	Finished bool
	// Meta and Err are set when Finished.
	Meta *channel.PictureMeta
	Err  error
}

// Downloader runs wallpaper downloads one at a time, in the background.
//
// Requests from a user -- the interface, the CLI, the tray -- are refused with
// ErrBusy while a download is running, because silently queueing them would
// apply a picture long after the user stopped waiting for it. A scheduled
// refresh is different: it is never refused, and one that arrives mid-download
// runs once the current one finishes, so a configuration change made during a
// download still takes effect.
type Downloader struct {
	store   store.Store
	refresh RefreshFunc
	reapply ReapplyFunc

	mu        sync.Mutex
	running   *Job
	startedAt time.Time
	pending   bool
	last      *Outcome
	listeners []func(DownloadEvent)
}

// NewDownloader builds a downloader. reapply may be nil, in which case
// re-applying is reported as unsupported.
func NewDownloader(s store.Store, refresh RefreshFunc, reapply ReapplyFunc) *Downloader {
	return &Downloader{store: s, refresh: refresh, reapply: reapply}
}

// OnChange registers a listener for downloads starting and finishing. It is
// called on the downloading goroutine, so it must not block for long.
func (d *Downloader) OnChange(fn func(DownloadEvent)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, fn)
}

// Status reports what the downloader is doing.
func (d *Downloader) Status() DownloadStatus {
	d.mu.Lock()
	defer d.mu.Unlock()

	status := DownloadStatus{State: StateIdle, Last: d.last}
	if d.running != nil {
		job := *d.running
		status.State = StateDownloading
		status.Job = &job
		status.StartedAt = d.startedAt.Unix()
	}
	return status
}

// Refresh starts a refresh for a user, returning ErrBusy if a download is
// already running. An empty channels slice uses the configured rotation;
// naming channels runs those instead, even if they are disabled. force
// downloads even a picture a channel applied last time.
func (d *Downloader) Refresh(channels []string, force bool) error {
	job := Job{Kind: JobRefresh, Channels: channels, Force: force}
	return d.start(job, func() (*channel.PictureMeta, error) {
		return d.refresh(channels, force)
	})
}

// Reapply starts downloading a recorded wallpaper again, returning ErrBusy if
// a download is already running.
//
// The entry is checked before anything starts, so a request for a missing
// entry or one with nothing to download gets an error it can act on rather
// than a download that fails in the background.
func (d *Downloader) Reapply(id int64) error {
	entry, err := d.store.WallpaperHistoryEntryByID(id)
	if err != nil {
		return err
	}
	if entry.URL == "" {
		return fmt.Errorf("history entry %d has no source URL to download again", id)
	}
	if d.reapply == nil {
		return errors.New("re-applying a wallpaper is not available here")
	}

	job := Job{Kind: JobReapply, EntryID: id}
	return d.start(job, func() (*channel.PictureMeta, error) {
		return d.reapply(entry)
	})
}

// Schedule starts a refresh for the timer or a configuration change. It is
// never refused: if a download is running, one refresh runs after it instead.
// Several requests arriving during one download still produce one refresh,
// since they all want the same thing.
func (d *Downloader) Schedule() {
	job := Job{Kind: JobRefresh, Scheduled: true}
	err := d.start(job, func() (*channel.PictureMeta, error) {
		return d.refresh(nil, false)
	})
	if errors.Is(err, ErrBusy) {
		d.mu.Lock()
		d.pending = true
		d.mu.Unlock()
		logrus.Debug("a download is running, refreshing again once it finishes")
	}
}

// start runs fn in the background unless a download is already running.
func (d *Downloader) start(job Job, fn func() (*channel.PictureMeta, error)) error {
	d.mu.Lock()
	if d.running != nil {
		d.mu.Unlock()
		return ErrBusy
	}
	d.running = &job
	d.startedAt = time.Now()
	d.mu.Unlock()

	d.notify(DownloadEvent{Job: job})

	go func() {
		meta, err := fn()

		outcome := &Outcome{Job: job, FinishedAt: time.Now().Unix()}
		if meta != nil {
			outcome.Result = resultOf(meta)
		}
		if err != nil {
			outcome.Error = err.Error()
		}

		d.mu.Lock()
		d.running = nil
		d.last = outcome
		runPending := d.pending
		d.pending = false
		d.mu.Unlock()

		d.notify(DownloadEvent{Job: job, Finished: true, Meta: meta, Err: err})

		if runPending {
			d.Schedule()
		}
	}()
	return nil
}

func (d *Downloader) notify(e DownloadEvent) {
	d.mu.Lock()
	listeners := append([]func(DownloadEvent){}, d.listeners...)
	d.mu.Unlock()

	for _, fn := range listeners {
		fn(e)
	}
}

func resultOf(meta *channel.PictureMeta) RefreshResult {
	return RefreshResult{
		Channel: meta.ChannelKey,
		Type:    meta.Channel,
		Title:   meta.Title,
		Credit:  meta.Credit,
	}
}
