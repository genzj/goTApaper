package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/service"
)

// Event types carried on the stream. Named constants so the server and its
// clients cannot drift on the spelling.
const (
	// EventLog is one log entry.
	EventLog = "log"
	// EventConfigChanged reports a setting, channel or layer having changed,
	// so a UI can reload rather than poll.
	EventConfigChanged = "config-changed"
	// EventDownloadStarted and EventDownloadFinished bracket every download
	// -- a refresh or a reapply, from whoever asked -- so a UI can show
	// progress for work it did not start itself, and learn how a request it
	// did start turned out.
	EventDownloadStarted  = "download-started"
	EventDownloadFinished = "download-finished"
)

// heartbeatInterval is how often a comment frame is sent on an idle stream.
//
// Without it an idle connection looks indistinguishable from a wedged one to
// anything between the client and the server, and the client cannot tell a
// quiet application from a dead one.
const heartbeatInterval = 25 * time.Second

// Event is one message on the stream.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// eventHub fans application events out to connected stream readers.
//
// It is separate from logging.Ring's own subscription because the stream
// carries more than logs: configuration changes and refresh progress arrive
// from elsewhere entirely, and a client wants them interleaved on one
// connection rather than having to open three.
type eventHub struct {
	mu          sync.RWMutex
	subscribers map[int]chan Event
	nextID      int
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: map[int]chan Event{}}
}

// Publish delivers an event to every connected reader.
//
// A reader that is not keeping up misses events rather than blocking the
// publisher: this is called from the refresh pipeline and from configuration
// writes, neither of which may be delayed by a slow UI.
func (h *eventHub) Publish(e Event) {
	h.mu.RLock()
	targets := make([]chan Event, 0, len(h.subscribers))
	for _, ch := range h.subscribers {
		targets = append(targets, ch)
	}
	h.mu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a channel of events and a function that ends the
// subscription.
func (h *eventHub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan Event, 64)
	h.subscribers[id] = ch
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers, id)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// Subscribers reports the number of connected readers.
func (h *eventHub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// Events returns the hub, so the embedding application can publish refresh
// progress and configuration changes into the stream.
func (s *Server) Events() interface{ Publish(Event) } {
	return s.handlers.events
}

// publishDownload forwards the downloader's progress onto the stream.
func (h *handlers) publishDownload(e service.DownloadEvent) {
	if !e.Finished {
		h.events.Publish(Event{Type: EventDownloadStarted, Data: e.Job})
		return
	}
	// The same shape as DownloadStatus.Last, so a client has one decoder for
	// both the event and the status it would otherwise poll.
	outcome := service.Outcome{Job: e.Job, FinishedAt: time.Now().Unix()}
	if e.Meta != nil {
		outcome.Result = service.RefreshResult{
			Channel: e.Meta.ChannelKey, Type: e.Meta.Channel,
			Title: e.Meta.Title, Credit: e.Meta.Credit,
		}
	}
	if e.Err != nil {
		outcome.Error = e.Err.Error()
	}
	h.events.Publish(Event{Type: EventDownloadFinished, Data: outcome})
}

// PublishEvent is the convenience form of Events().Publish.
func (s *Server) PublishEvent(eventType string, data any) {
	s.handlers.events.Publish(Event{Type: eventType, Data: data})
}

// registerEventRoutes serves the event stream: one stream carrying log lines,
// configuration changes and refresh progress. A single endpoint because it
// works unchanged in the webview, in a browser and under curl, which is what
// keeps the UI from needing a transport the CLI does not have.
func (h *handlers) registerEventRoutes(r *router) {
	r.handle("GET", "/events", h.streamEvents)
}

// streamEvents streams server-sent events until the client disconnects.
func (h *handlers) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Every net/http ResponseWriter flushes, so this is a programming
		// error rather than a client one.
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Proxies are not expected on loopback, but a buffering one would defeat
	// the whole endpoint, and the header costs nothing.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, cancelEvents := h.events.Subscribe()
	defer cancelEvents()

	// Log entries are picked up from the ring rather than republished into the
	// hub, so that every logrus call does not have to know an API server
	// exists.
	logs, cancelLogs := h.logs.Follow(256)
	defer cancelLogs()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case e, open := <-events:
			if !open {
				return
			}
			if !writeEvent(w, flusher, e) {
				return
			}

		case entry, open := <-logs:
			if !open {
				return
			}
			if !writeEvent(w, flusher, Event{Type: EventLog, Data: entry}) {
				return
			}

		case <-heartbeat.C:
			// A comment frame: ignored by EventSource, but it proves the
			// connection is alive.
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeEvent writes one SSE frame, reporting whether the connection is still
// usable.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, e Event) bool {
	payload, err := json.Marshal(e.Data)
	if err != nil {
		logrus.WithField("event", e.Type).WithError(err).Debug(
			"cannot encode an event for the stream",
		)
		return true
	}

	// The event name goes in the SSE "event:" field so a client can filter
	// with addEventListener, and the payload is one line because JSON encoding
	// never emits a raw newline.
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, payload); err != nil {
		return false
	}
	flusher.Flush()
	return true
}
