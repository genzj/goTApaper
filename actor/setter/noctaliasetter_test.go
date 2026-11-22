//go:build linux

package setter

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/genzj/goTApaper/config"
)

func TestNoctaliaSetterRegistered(t *testing.T) {
	if _, ok := Setters.Get("noctalia"); !ok {
		t.Fatal("noctalia setter is not registered")
	}

	def, ok := config.Lookup(config.KeyWallpaperSetter)
	if !ok {
		t.Fatal("the setter setting has no definition")
	}
	if err := def.ValidateValue("noctalia"); err != nil {
		t.Fatalf("noctalia is not accepted by %s: %v", config.KeyWallpaperSetter, err)
	}
}

func TestNoctaliaSetterAlternatesSlotsAndCleansPrevious(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "wallpaper.jpeg")
	if err := os.WriteFile(src, []byte("image-v1"), 0o600); err != nil {
		t.Fatalf("write source wallpaper: %v", err)
	}

	origRun := noctaliaRunCommand
	t.Cleanup(func() {
		noctaliaRunCommand = origRun
		noctaliaSlotMu.Lock()
		noctaliaLastSlot = ""
		noctaliaSlotMu.Unlock()
	})
	noctaliaSlotMu.Lock()
	noctaliaLastSlot = ""
	noctaliaSlotMu.Unlock()

	var (
		currentWallpaper = "/some/other/wallpaper.jpg"
		calls            [][]string
	)
	noctaliaRunCommand = func(path string, arg ...string) (string, error) {
		call := append([]string{path}, arg...)
		calls = append(calls, call)
		if len(arg) >= 2 && arg[0] == "msg" && arg[1] == "wallpaper-get" {
			return currentWallpaper + "\n", nil
		}
		if len(arg) >= 3 && arg[0] == "msg" && arg[1] == "wallpaper-set" {
			currentWallpaper = arg[2]
			return "ok\n", nil
		}
		return "", errors.New("unexpected command")
	}

	s := NoctaliaSetter(0)

	// First set should pick slot 0.
	if err := s.Set(src); err != nil {
		t.Fatalf("first Set failed: %v", err)
	}
	slot0 := filepath.Join(dir, noctaliaSlotPrefix+"0.jpeg")
	slot1 := filepath.Join(dir, noctaliaSlotPrefix+"1.jpeg")

	if currentWallpaper != slot0 {
		t.Fatalf("first Set applied %q, want %q", currentWallpaper, slot0)
	}
	gotBytes, err := os.ReadFile(slot0)
	if err != nil {
		t.Fatalf("read slot0: %v", err)
	}
	if string(gotBytes) != "image-v1" {
		t.Fatalf("slot0 contents = %q, want %q", gotBytes, "image-v1")
	}

	// Overwrite source with a new image; second set must pick slot 1 and remove slot 0.
	if err := os.WriteFile(src, []byte("image-v2"), 0o600); err != nil {
		t.Fatalf("overwrite source wallpaper: %v", err)
	}
	if err := s.Set(src); err != nil {
		t.Fatalf("second Set failed: %v", err)
	}
	if currentWallpaper != slot1 {
		t.Fatalf("second Set applied %q, want %q", currentWallpaper, slot1)
	}
	gotBytes, err = os.ReadFile(slot1)
	if err != nil {
		t.Fatalf("read slot1: %v", err)
	}
	if string(gotBytes) != "image-v2" {
		t.Fatalf("slot1 contents = %q, want %q", gotBytes, "image-v2")
	}
	if _, err := os.Stat(slot0); !os.IsNotExist(err) {
		t.Fatalf("expected previous slot %q to be removed, stat err = %v", slot0, err)
	}

	wantCalls := [][]string{
		{"noctalia", "msg", "wallpaper-get"},
		{"noctalia", "msg", "wallpaper-set", slot0},
		{"noctalia", "msg", "wallpaper-get"},
		{"noctalia", "msg", "wallpaper-set", slot1},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("commands = %v, want %v", calls, wantCalls)
	}
}

func TestNoctaliaSetterPropagatesSetError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "wallpaper.png")
	if err := os.WriteFile(src, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("write source wallpaper: %v", err)
	}

	origRun := noctaliaRunCommand
	t.Cleanup(func() {
		noctaliaRunCommand = origRun
		noctaliaSlotMu.Lock()
		noctaliaLastSlot = ""
		noctaliaSlotMu.Unlock()
	})
	noctaliaSlotMu.Lock()
	noctaliaLastSlot = ""
	noctaliaSlotMu.Unlock()

	noctaliaRunCommand = func(path string, arg ...string) (string, error) {
		if len(arg) >= 2 && arg[1] == "wallpaper-get" {
			return "", errors.New("ipc unavailable")
		}
		return "error: noctalia not running\n", errors.New("exit status 1")
	}

	err := NoctaliaSetter(0).Set(src)
	if err == nil {
		t.Fatal("expected error when wallpaper-set fails, got nil")
	}
	if !strings.Contains(err.Error(), "noctalia not running") {
		t.Fatalf("error %q should include command output", err.Error())
	}
}
