package util

import (
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileURLRoundTrip(t *testing.T) {
	path, err := filepath.Abs(filepath.Join(t.TempDir(), "a picture.jpg"))
	if err != nil {
		t.Fatal(err)
	}

	raw := PathFileURL(path)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("PathFileURL(%q) = %q does not parse: %v", path, raw, err)
	}
	if u.Scheme != "file" {
		t.Fatalf("scheme = %q", u.Scheme)
	}
	if got := FileURLPath(u); got != path {
		t.Fatalf("round trip = %q, want %q", got, path)
	}
}

func TestFileURLPathOnWindowsDropsTheSlashBeforeTheDrive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are Windows-only")
	}
	u, _ := url.Parse("file:///C:/Users/me/pic.jpg")
	if got, want := FileURLPath(u), `C:\Users\me\pic.jpg`; got != want {
		t.Fatalf("FileURLPath = %q, want %q", got, want)
	}
}
