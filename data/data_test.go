package data

import (
	"io"
	"testing"
)

// The callers open the tray icons with a leading slash and the example
// configuration without one, as vfsgen accepted; both must keep working.
func TestExampleAssetsServeEveryFileTheCallersOpen(t *testing.T) {
	for _, name := range []string{
		"/assets/icons8-sheet-of-paper-systray.ico",
		"/assets/icons8-sheet-of-paper-template.ico",
		"config.yaml.example",
	} {
		f, err := ExampleAssets.Open(name)
		if err != nil {
			t.Errorf("Open(%q): %v", name, err)
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil || len(data) == 0 {
			t.Errorf("Open(%q): read %d bytes, err %v", name, len(data), err)
		}
	}
}
