package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesListsTheActiveFileFirstThenRotatedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "gotapaper.log")

	// lumberjack names a backup by inserting a timestamp before the extension.
	write := func(name string, age time.Duration) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		stamp := time.Now().Add(-age)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("Chtimes %s: %v", name, err)
		}
		return path
	}

	write("gotapaper.log", 0)
	write("gotapaper-2026-10-23T09-00-00.000.log", 2*time.Hour)
	write("gotapaper-2026-10-22T09-00-00.000.log", 48*time.Hour)
	// Neither of these belongs to this log: one is a different log, the other
	// is not a log at all.
	write("other.log", time.Hour)
	write("gotapaper.db", time.Hour)

	files, err := Files(active)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	want := []string{
		"gotapaper.log",
		"gotapaper-2026-10-23T09-00-00.000.log",
		"gotapaper-2026-10-22T09-00-00.000.log",
	}
	if !equal(names, want) {
		t.Fatalf("Files() = %v, want %v", names, want)
	}
	if !files[0].Active {
		t.Error("the active file is not marked Active")
	}
	if files[1].Active {
		t.Error("a rotated file is marked Active")
	}
	if files[0].Size != 1 {
		t.Errorf("Size = %d, want 1", files[0].Size)
	}
}

func TestFilesOnAMissingDirectoryReturnsNothing(t *testing.T) {
	// Nothing has been logged to disk yet. An empty list is the honest answer,
	// not an error the caller has to special-case.
	files, err := Files(filepath.Join(t.TempDir(), "absent", "gotapaper.log"))
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("Files() = %v, want empty", files)
	}
}

func TestFilesWithNoPathReturnsNothing(t *testing.T) {
	files, err := Files("")
	if err != nil || len(files) != 0 {
		t.Fatalf("Files(\"\") = %v, %v; want empty, nil", files, err)
	}
}

func TestNewRotatingWriterCreatesTheLogDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "nested", "gotapaper.log")

	writer, err := NewRotatingWriter(RotateOptions{Path: path})
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	defer writer.Close()

	if _, err := writer.Write([]byte("line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the log file was not created: %v", err)
	}
}
