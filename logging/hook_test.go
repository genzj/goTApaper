package logging

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// newTestLogger returns a logger wired to a fresh hook, writing nowhere else.
func newTestLogger(t *testing.T) (*logrus.Logger, *Hook) {
	t.Helper()

	hook := NewHook(NewRing(100))
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.DebugLevel)
	logger.AddHook(hook)
	return logger, hook
}

func TestHookCapturesMessageLevelAndFields(t *testing.T) {
	logger, hook := newTestLogger(t)

	logger.WithField("channel", "bing").WithError(errors.New("boom")).Warn("could not download")

	entries := hook.Ring().Snapshot(Filter{})
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want 1", len(entries))
	}
	e := entries[0]

	if e.Message != "could not download" {
		t.Errorf("Message = %q", e.Message)
	}
	if e.Level != "warning" {
		t.Errorf("Level = %q, want \"warning\"", e.Level)
	}
	if got := e.Fields["channel"]; got != "bing" {
		t.Errorf("Fields[channel] = %q, want \"bing\"", got)
	}
	// An error field must arrive as its message, not as a Go struct dump.
	if got := e.Fields[logrus.ErrorKey]; got != "boom" {
		t.Errorf("Fields[error] = %q, want \"boom\"", got)
	}
	if e.Time.IsZero() {
		t.Error("Time is zero")
	}
}

func TestHookRespectsTheLoggerLevel(t *testing.T) {
	logger, hook := newTestLogger(t)
	logger.SetLevel(logrus.InfoLevel)

	logger.Debug("invisible")
	logger.Info("visible")

	entries := hook.Ring().Snapshot(Filter{})
	if len(entries) != 1 || entries[0].Message != "visible" {
		t.Fatalf("captured %v, want only the info entry", messages(entries))
	}
}

func TestHookDoesNotRetainTheLogrusEntry(t *testing.T) {
	logger, hook := newTestLogger(t)

	// logrus reuses entry objects between calls. If the hook stored a
	// reference, the second call would rewrite the first captured record.
	logger.WithField("n", 1).Info("first")
	logger.WithField("n", 2).Info("second")

	entries := hook.Ring().Snapshot(Filter{})
	if len(entries) != 2 {
		t.Fatalf("captured %d entries, want 2", len(entries))
	}
	if entries[0].Message != "first" || entries[0].Fields["n"] != "1" {
		t.Fatalf("the first entry was mutated: %+v", entries[0])
	}
}

func TestHookWritesJSONLinesToTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "test.log")

	logger, hook := newTestLogger(t)
	writer, err := NewRotatingWriter(RotateOptions{Path: path, MaxSizeMB: 1, MaxBackups: 1})
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	hook.SetFile(writer)

	logger.WithField("channel", "bing").Info("downloaded")

	if err := hook.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the log file was not created: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 1 {
		t.Fatalf("wrote %d lines, want 1: %q", len(lines), content)
	}

	var decoded Entry
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("the line is not valid JSON: %v (%q)", err, lines[0])
	}
	if decoded.Message != "downloaded" || decoded.Fields["channel"] != "bing" {
		t.Fatalf("decoded = %+v", decoded)
	}
	// The file record must carry the same sequence number the ring assigned,
	// so the two views of one entry can be correlated.
	if decoded.Seq == 0 {
		t.Error("Seq was not recorded in the file")
	}
}

func TestHookWithNoFileStillCapturesToTheRing(t *testing.T) {
	logger, hook := newTestLogger(t)

	logger.Info("in memory only")

	if got := hook.Ring().Len(); got != 1 {
		t.Fatalf("ring holds %d entries, want 1", got)
	}
}

func TestHookSetFileClosesThePreviousWriter(t *testing.T) {
	dir := t.TempDir()
	hook := NewHook(NewRing(10))

	first := &countingCloser{}
	hook.SetFile(first)

	second, err := NewRotatingWriter(RotateOptions{Path: filepath.Join(dir, "second.log")})
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	hook.SetFile(second)

	// Re-applying the logging settings must not leak the old file handle.
	if first.closes != 1 {
		t.Fatalf("the previous writer was closed %d times, want 1", first.closes)
	}
	_ = hook.Close()
}

func TestHookSetRingReplacesTheBuffer(t *testing.T) {
	logger, hook := newTestLogger(t)
	logger.Info("old")

	hook.SetRing(NewRing(5))

	if got := hook.Ring().Cap(); got != 5 {
		t.Fatalf("Cap() = %d, want 5", got)
	}
	if got := hook.Ring().Len(); got != 0 {
		t.Fatalf("the new ring holds %d entries, want 0", got)
	}
}

func TestNewRotatingWriterWithNoPathDisablesFileLogging(t *testing.T) {
	writer, err := NewRotatingWriter(RotateOptions{})
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	if writer != nil {
		t.Fatal("an empty path returned a writer, want nil")
	}
}

type countingCloser struct {
	closes int
}

func (c *countingCloser) Write(p []byte) (int, error) { return len(p), nil }

func (c *countingCloser) Close() error {
	c.closes++
	return nil
}
