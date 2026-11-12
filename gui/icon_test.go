package gui

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/genzj/goTApaper/examples"
)

func TestICOToPNGExtractsDecodablePNGFromBundledIcons(t *testing.T) {
	for _, name := range []string{
		"icons8-sheet-of-paper-systray.ico",
		"icons8-sheet-of-paper-template.ico",
	} {
		ico, err := examples.FS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		extracted := icoToPNG(ico)
		img, err := png.Decode(bytes.NewReader(extracted))
		if err != nil {
			t.Fatalf("png.Decode(icoToPNG(%s)): %v", name, err)
		}
		bounds := img.Bounds()
		if bounds.Dx() != 64 || bounds.Dy() != 64 {
			t.Fatalf("%s: got %dx%d, want 64x64 largest frame", name, bounds.Dx(), bounds.Dy())
		}
	}
}

func TestICOToPNGPassesNonICOInputThrough(t *testing.T) {
	raw := []byte("not an ico file")
	if got := icoToPNG(raw); !bytes.Equal(got, raw) {
		t.Fatalf("got %q, want %q unchanged", got, raw)
	}
}
