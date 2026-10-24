package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/logging"
	"github.com/genzj/goTApaper/service"
)

func TestLogsRouteServesTheRingBuffer(t *testing.T) {
	h, _ := newTestServer(t)

	// The log service reads the process-wide hook, which is what config
	// InitLogger installs in a real run.
	logging.Default().Ring().Add(logging.Entry{
		Time: time.Now(), Level: "info", Message: "something happened",
	})

	var page service.LogPage
	rec := getJSON(t, h, "/api/v1/logs", &page)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	if page.Capacity == 0 {
		t.Error("Capacity = 0, so a client cannot tell a gap from a quiet period")
	}
	if len(page.Entries) == 0 {
		t.Fatal("no entries returned")
	}
	if page.LastSeq == 0 {
		t.Error("LastSeq = 0, so a client cannot poll for what came after")
	}
}

func TestLogsRouteFiltersByLevelAndSequence(t *testing.T) {
	h, _ := newTestServer(t)

	ring := logging.Default().Ring()
	ring.Add(logging.Entry{Level: "debug", Message: "noise"})
	marker := ring.Add(logging.Entry{Level: "error", Message: "a failure"})

	var page service.LogPage
	getJSON(t, h, "/api/v1/logs?level=error", &page)
	for _, e := range page.Entries {
		if e.Level == "debug" {
			t.Fatalf("a debug entry survived level=error: %+v", e)
		}
	}

	var after service.LogPage
	getJSON(t, h, "/api/v1/logs?since="+itoa(int64(marker.Seq)), &after)
	for _, e := range after.Entries {
		if e.Seq <= marker.Seq {
			t.Fatalf("entry %d is not newer than since=%d", e.Seq, marker.Seq)
		}
	}
}

func TestLogsRouteRejectsAnUnparseableSince(t *testing.T) {
	h, _ := newTestServer(t)

	// Unlike limit, a bad "since" cannot be defaulted silently: returning
	// everything when the client asked for a tail would look like a flood.
	if rec := getJSON(t, h, "/api/v1/logs?since=abc", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLogsRouteToleratesAnUnparseableLimit(t *testing.T) {
	h, _ := newTestServer(t)

	// A display hint, so a default page beats an error the client cannot act
	// on.
	if rec := getJSON(t, h, "/api/v1/logs?limit=abc", nil); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestLogFilesRouteListsFiles(t *testing.T) {
	h, _ := newTestServer(t)

	// No log file is configured in a test, so an empty list is the expected
	// answer rather than an error.
	var files []logging.FileInfo
	if rec := getJSON(t, h, "/api/v1/logs/files", &files); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogFileDownloadRejectsAPathTraversal(t *testing.T) {
	h, _ := newTestServer(t)

	// The API is authenticated, but a traversal is still a traversal, and the
	// listing is the authoritative set of openable files.
	for _, name := range []string{
		"..%2f..%2f..%2f..%2fetc%2fpasswd",
		"%2eetc%2fpasswd",
		"nonexistent.log",
	} {
		rec := getJSON(t, h, "/api/v1/logs/files/"+name, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rec.Code)
		}
	}
}

func TestEventStreamDeliversLogEntriesAndPublishedEvents(t *testing.T) {
	h, _ := newTestServer(t)

	// httptest.ResponseRecorder does not stream, so the stream is driven on a
	// real connection instead.
	srv := httptest.NewServer(h)
	defer srv.Close()

	streamReq, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	streamReq.Header.Set("Authorization", "Bearer "+testToken)

	resp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}

	// The subscription is registered asynchronously, so wait for it rather
	// than racing it.
	deadline := time.Now().Add(2 * time.Second)
	for logging.Default().Ring().Subscribers() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	logrus.StandardLogger().AddHook(logging.Default())
	logging.Default().Ring().Add(logging.Entry{Level: "info", Message: "streamed"})

	frames := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		var block strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if block.Len() > 0 {
					frames <- block.String()
					block.Reset()
				}
				continue
			}
			block.WriteString(line)
			block.WriteString("\n")
		}
	}()

	select {
	case frame := <-frames:
		if !strings.Contains(frame, "event: "+EventLog) {
			t.Fatalf("frame is not a log event: %q", frame)
		}
		if !strings.Contains(frame, "streamed") {
			t.Fatalf("frame does not carry the message: %q", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a stream frame")
	}
}

func TestEventStreamRequiresAToken(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestEventHubDropsForASlowReaderRatherThanBlocking(t *testing.T) {
	hub := newEventHub()
	_, cancel := hub.Subscribe()
	defer cancel()

	// Publish is called from the refresh pipeline and from configuration
	// writes, neither of which may be delayed by a UI that stopped reading.
	done := make(chan struct{})
	go func() {
		for range 1000 {
			hub.Publish(Event{Type: EventConfigChanged})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a subscriber that is not reading")
	}
}

func TestEventHubCancelIsIdempotent(t *testing.T) {
	hub := newEventHub()
	ch, cancel := hub.Subscribe()

	cancel()
	cancel()

	if _, open := <-ch; open {
		t.Fatal("the channel yielded a value after cancel")
	}
	if got := hub.Subscribers(); got != 0 {
		t.Fatalf("Subscribers() = %d, want 0", got)
	}
}

func TestWriteEventEncodesOneFramePerEvent(t *testing.T) {
	rec := httptest.NewRecorder()

	if !writeEvent(rec, rec, Event{Type: EventRefreshFinished, Data: map[string]string{
		"channel": "bing",
	}}) {
		t.Fatal("writeEvent reported a dead connection")
	}

	body := rec.Body.String()
	if !strings.HasPrefix(body, "event: "+EventRefreshFinished+"\n") {
		t.Errorf("frame does not start with the event name: %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("frame is not terminated by a blank line: %q", body)
	}
	// The payload must stay on one line, or a client sees a truncated frame.
	data := strings.TrimPrefix(body, "event: "+EventRefreshFinished+"\n")
	data = strings.TrimSuffix(data, "\n\n")
	if strings.Count(data, "\n") != 0 {
		t.Errorf("payload spans multiple lines: %q", data)
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &decoded); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if decoded["channel"] != "bing" {
		t.Errorf("decoded = %v", decoded)
	}
}
