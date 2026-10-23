//go:build darwin
// +build darwin

package setter

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"howett.net/plist"
)

// choiceRequest builds one "Your Photos" index element the way macOS stores
// it: a binary plist nested inside the array of the outer one.
func choiceRequest(t *testing.T, originalPath, copyPath, bundle string) []byte {
	t.Helper()

	entry := map[string]interface{}{
		"originatingBundleIdentifier": bundle,
		"originalURL":                 map[string]interface{}{"relative": "file://" + originalPath},
		"copyURL":                     map[string]interface{}{"relative": "file://" + copyPath},
	}
	raw, err := plist.Marshal(entry, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("cannot encode a test index entry: %s", err)
	}
	return raw
}

// TestDecodeChoiceRequestOwnership pins which entries goTApaper claims.
//
// Ownership decides what gets deleted, and the bundle identifier alone cannot
// decide it: every AppleScript-driven wallpaper tool reports
// com.apple.systemevents, so matching on it would also claim a competing
// tool's entries. The requested file's location is the deciding test.
func TestDecodeChoiceRequestOwnership(t *testing.T) {
	appDir := t.TempDir()

	for _, tc := range []struct {
		name      string
		original  string
		bundle    string
		wantOwned bool
	}{
		{
			name:      "our own refresh",
			original:  filepath.Join(appDir, "wallpaper_osx_0.jpeg"),
			bundle:    appleScriptBundle,
			wantOwned: true,
		},
		{
			name:     "another AppleScript tool",
			original: "/tmp/some-other-tool/picture.jpeg",
			bundle:   appleScriptBundle,
			// Neither our directory nor our filename.
			wantOwned: false,
		},
		{
			// Written by an earlier run under a different --app-dir. Still
			// ours to clean up, recognised by the filename we always use.
			name:      "our filename, a directory we no longer use",
			original:  "/var/folders/xx/T/tmp.abc123/wallpaper_osx_1.jpeg",
			bundle:    appleScriptBundle,
			wantOwned: true,
		},
		{
			name:      "added by hand in System Settings",
			original:  filepath.Join(appDir, "wallpaper_osx_0.jpeg"),
			bundle:    "com.apple.Wallpaper-Settings.extension",
			wantOwned: false,
		},
		{
			// A sibling directory whose name merely starts with the app
			// directory's must not be mistaken for being inside it. The
			// filename here is deliberately not one of ours, so the
			// directory test is what decides.
			name:      "app directory prefix, not inside it",
			original:  appDir + "-elsewhere/some-photo.jpeg",
			bundle:    appleScriptBundle,
			wantOwned: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := choiceRequest(t, tc.original, "/tmp/copy.jpeg", tc.bundle)

			got := decodeChoiceRequest(raw, appDir)
			if got.Owned != tc.wantOwned {
				t.Errorf("Owned = %v, want %v for %s", got.Owned, tc.wantOwned, tc.original)
			}
			if got.OriginalURL != "file://"+tc.original {
				t.Errorf("OriginalURL = %q, want %q", got.OriginalURL, "file://"+tc.original)
			}
		})
	}
}

// TestDecodeChoiceRequestUnreadable covers an entry written by a macOS release
// whose format we do not recognise. It must come back unowned, so a prune
// leaves it alone rather than deleting state it could not read.
func TestDecodeChoiceRequestUnreadable(t *testing.T) {
	got := decodeChoiceRequest([]byte("not a plist"), t.TempDir())

	if got.Owned {
		t.Error("an unreadable entry was claimed as ours")
	}
	if got.OriginalURL != "" {
		t.Errorf("OriginalURL = %q, want empty", got.OriginalURL)
	}
}

// TestLocalPath checks the file:// handling that gates every deletion.
func TestLocalPath(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		wantPath string
		wantOK   bool
	}{
		{raw: "file:///Users/me/.goTApaper/a.jpeg", wantPath: "/Users/me/.goTApaper/a.jpeg", wantOK: true},
		{raw: "file:///tmp/./a.jpeg", wantPath: "/tmp/a.jpeg", wantOK: true},
		{raw: "", wantOK: false},
		{raw: "https://example.com/a.jpeg", wantOK: false},
		{raw: "file://", wantOK: false},
	} {
		path, ok := localPath(tc.raw)
		if ok != tc.wantOK {
			t.Errorf("localPath(%q) ok = %v, want %v", tc.raw, ok, tc.wantOK)
			continue
		}
		if ok && path != tc.wantPath {
			t.Errorf("localPath(%q) = %q, want %q", tc.raw, path, tc.wantPath)
		}
	}
}

// TestRenderPrefixMatchesTheObservedNaming pins the naming rule the whole
// attribution rests on.
//
// The expected digest below was taken from a real bitmap in the agent's cache
// after setting that exact path, so this test fails if the derivation drifts
// from what macOS actually writes.
func TestRenderPrefixMatchesTheObservedNaming(t *testing.T) {
	const (
		path = "/Users/zhuje/.goTApaper/wallpaper_osx_0.jpeg"
		want = "5a9855e4b3851385ff499855efa33f639e17d7a9b9006de67a0a17365099ac01"
	)

	if got := renderPrefix(path); got != want {
		t.Errorf("renderPrefix(%q) = %q, want %q", path, got, want)
	}

	// And the prefix must be recoverable from a real filename, which carries
	// the display geometry and the set time after it.
	name := want + "-4112-2658-0-41c8459125f6231a.bmp"
	if got := renderPrefixOf(filepath.Join("/somewhere", name)); got != want {
		t.Errorf("renderPrefixOf(%q) = %q, want %q", name, got, want)
	}
	if got := renderPrefixOf("/somewhere/nodashes.bmp"); got != "" {
		t.Errorf("renderPrefixOf on an unexpected name = %q, want empty", got)
	}
}

// renderFor writes a bitmap named the way the agent names the renders of
// originalPath, and returns its full path.
func renderFor(t *testing.T, renderDir, originalPath, setTime string, size int) string {
	t.Helper()

	path := filepath.Join(
		renderDir, renderPrefix(originalPath)+"-4112-2658-0-"+setTime+".bmp",
	)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPlanRendersForEntriesAttributesByName is the point of the change: a
// bitmap is matched to the image it renders by name, so renders of wallpapers
// that stay are kept exactly rather than guessed at by age.
func TestPlanRendersForEntriesAttributesByName(t *testing.T) {
	home := t.TempDir()
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = os.UserHomeDir })

	renderDir := agentRenderDir()
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const (
		dropped  = "/Users/me/.goTApaper/wallpaper_osx_0.jpeg"
		survivor = "/Users/me/Pictures/my-own-wallpaper.jpeg"
		vanished = "/Users/me/.goTApaper/wallpaper_osx_1765829621.jpeg"
	)

	// Two renders for the dropped entry: one image, applied twice.
	droppedRenders := []string{
		renderFor(t, renderDir, dropped, "41c8459125f6231a", 10),
		renderFor(t, renderDir, dropped, "41c8459234af4ffa", 10),
	}
	// The survivor's render is written last, so an age-based rule would have
	// kept it for the wrong reason. Written first here to prove the name is
	// what decides.
	survivorRender := renderFor(t, renderDir, survivor, "41c8459332e261c1", 10)
	orphanRender := renderFor(t, renderDir, vanished, "41c845935aaaa27b", 10)

	// Not a bitmap, so not ours to touch.
	if err := os.WriteFile(filepath.Join(renderDir, "cacheVersion.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	stats := PruneStats{Entries: []PruneEntry{{OriginalURL: "file://" + dropped}}}
	planRendersForEntries(&stats, map[string]bool{renderPrefix(survivor): true}, true)

	if stats.RendersKept != 1 {
		t.Errorf("RendersKept = %d, want 1 (the surviving wallpaper's render)", stats.RendersKept)
	}
	for _, path := range stats.Renders {
		if path == survivorRender {
			t.Error("a surviving wallpaper's render was planned for removal")
		}
	}
	if len(stats.Renders) != 3 {
		t.Fatalf("planned %d renders, want 3: %v", len(stats.Renders), stats.Renders)
	}
	if want := int64(30); stats.RenderBytes != want {
		t.Errorf("RenderBytes = %d, want %d", stats.RenderBytes, want)
	}

	// Both of the dropped image's renders attach to its entry, so the listing
	// can show what goes with what.
	got := stats.Entries[0].Renders
	if len(got) != 2 {
		t.Errorf("entry got %d renders, want 2: %v", len(got), got)
	}
	for _, want := range droppedRenders {
		if !slices.Contains(got, want) {
			t.Errorf("render %s was not attributed to its entry", filepath.Base(want))
		}
	}

	// The render of an image no longer in the index can only be an orphan.
	if len(stats.OrphanRenders) != 1 || stats.OrphanRenders[0] != orphanRender {
		t.Errorf("OrphanRenders = %v, want just %s", stats.OrphanRenders, orphanRender)
	}
	if want := int64(10); stats.OrphanBytes != want {
		t.Errorf("OrphanBytes = %d, want %d", stats.OrphanBytes, want)
	}
}

// TestPlanRendersForEntriesKeepsOnlyTheCurrentWhenItCannotAttribute covers the
// fallback: with the index unreadable there is nothing to match names against,
// so it keeps the newest bitmap and no more. Losing the index must not mean
// deleting the render of the wallpaper on screen.
func TestPlanRendersForEntriesKeepsOnlyTheCurrentWhenItCannotAttribute(t *testing.T) {
	home := t.TempDir()
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = os.UserHomeDir })

	renderDir := agentRenderDir()
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var newest string
	for i, name := range []string{"old.bmp", "older.bmp", "newest.bmp"} {
		path := filepath.Join(renderDir, name)
		if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
		newest = path
	}

	stats := PruneStats{}
	planRendersForEntries(&stats, nil, false)

	if stats.RendersKept != 1 {
		t.Errorf("RendersKept = %d, want 1", stats.RendersKept)
	}
	if len(stats.Renders) != 2 {
		t.Fatalf("planned %d renders, want 2: %v", len(stats.Renders), stats.Renders)
	}
	if slices.Contains(stats.Renders, newest) {
		t.Error("the newest render was planned for removal with no index to attribute by")
	}
	if want := int64(20); stats.RenderBytes != want {
		t.Errorf("RenderBytes = %d, want %d", stats.RenderBytes, want)
	}
}

// TestPlanRendersForEntriesWithOnlyTheCurrentRender covers the steady state
// once the caches have been cleaned: a single bitmap on disk, belonging to the
// wallpaper on screen, so nothing is planned.
//
// The kept count is what lets the cleanup command say so -- reporting only
// "0 to remove" next to a bitmap the user can see looks like the pruner missed
// it.
func TestPlanRendersForEntriesWithOnlyTheCurrentRender(t *testing.T) {
	home := t.TempDir()
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = os.UserHomeDir })

	renderDir := agentRenderDir()
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const current = "/Users/me/.goTApaper/wallpaper_osx_1.jpeg"
	renderFor(t, renderDir, current, "41c8459125f6231a", 16)

	stats := PruneStats{}
	planRendersForEntries(&stats, map[string]bool{renderPrefix(current): true}, true)

	if len(stats.Renders) != 0 {
		t.Errorf("planned %v, want nothing: the only bitmap is the current render", stats.Renders)
	}
	if stats.RenderBytes != 0 {
		t.Errorf("RenderBytes = %d, want 0", stats.RenderBytes)
	}
	if stats.RendersKept != 1 {
		t.Errorf("RendersKept = %d, want 1", stats.RendersKept)
	}
}

// TestPruneDryRunChangesNothing is the property the --dry-run flag sells: a
// populated plan, and not a byte altered on disk.
func TestPruneDryRunChangesNothing(t *testing.T) {
	home := t.TempDir()
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = os.UserHomeDir })

	renderDir := agentRenderDir()
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two renders of an image the (unreadable, under this fake home) index
	// knows nothing about, so the age fallback applies and one is planned.
	for _, name := range []string{"a.bmp", "b.bmp"} {
		if err := os.WriteFile(filepath.Join(renderDir, name), []byte("12345"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := PruneWallpaperCaches(PruneOptions{AppDir: t.TempDir(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run failed: %s", err)
	}

	if !stats.DryRun {
		t.Error("stats do not report a dry run")
	}
	if len(stats.Renders) != 1 {
		t.Errorf("planned %d renders, want 1 (the other is the newest)", len(stats.Renders))
	}

	for _, name := range []string{"a.bmp", "b.bmp"} {
		if _, err := os.Stat(filepath.Join(renderDir, name)); err != nil {
			t.Errorf("a dry run removed %s", name)
		}
	}
}

// TestProtectedPathsIncludesTheCallersPaths guards the race that made a prune
// delete the wallpaper it had just applied.
//
// `set picture` returns before the agent has switched, so asking the system
// which wallpaper is current straight afterwards still names the previous one.
// A caller that knows what it set must be able to say so, and that path must
// be protected whether or not the system agrees yet.
func TestProtectedPathsIncludesTheCallersPaths(t *testing.T) {
	justSet := filepath.Join(t.TempDir(), "wallpaper_osx_0.jpeg")

	got := protectedPaths([]string{justSet})

	if !got[justSet] {
		t.Errorf("the path the caller just set is not protected: %v", got)
	}

	// A relative path must land in the set as an absolute one, since that is
	// what the entries are compared as.
	dir := t.TempDir()
	t.Chdir(dir)
	if got := protectedPaths([]string{"relative.jpeg"}); !got[filepath.Join(dir, "relative.jpeg")] {
		t.Errorf("a relative path was not resolved before protecting it: %v", got)
	}

	// An empty path is not a wildcard.
	if got := protectedPaths([]string{""}); got[""] {
		t.Error("an empty path was added to the protected set")
	}
}

// TestCurrentSlot pins which of several desktops' wallpapers is treated as the
// one a new wallpaper has to differ from.
func TestCurrentSlot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
		want  string
	}{
		{
			name:  "every desktop agrees",
			paths: []string{"/app/wallpaper_osx_0.jpeg", "/app/wallpaper_osx_0.jpeg"},
			want:  "/app/wallpaper_osx_0.jpeg",
		},
		{
			// Ours is what matters, wherever it appears in the list: that is
			// the path the next refresh must not reuse.
			name:  "ours is not first",
			paths: []string{"/Users/me/Pictures/mine.jpeg", "/app/wallpaper_osx_1.jpeg"},
			want:  "/app/wallpaper_osx_1.jpeg",
		},
		{
			// Right after the user sets a wallpaper by hand, none is ours.
			name:  "none is ours",
			paths: []string{"/Users/me/Pictures/mine.jpeg", "/Users/me/Pictures/other.jpeg"},
			want:  "/Users/me/Pictures/mine.jpeg",
		},
		{name: "nothing reported", paths: nil, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := currentSlot(tc.paths); got != tc.want {
				t.Errorf("currentSlot(%v) = %q, want %q", tc.paths, got, tc.want)
			}
		})
	}
}

// TestNextSlotAvoidsTheCurrentOne is the property the whole two-slot scheme
// rests on: the path handed to macOS must differ from the one on screen, or
// the wallpaper will not be re-rendered at all.
func TestNextSlotAvoidsTheCurrentOne(t *testing.T) {
	const dir = "/app"

	zero, err := filepath.Abs(filepath.Join(dir, wallpaperSlots[0]+".jpeg"))
	if err != nil {
		t.Fatal(err)
	}

	if got := nextSlot(zero, dir, ".jpeg"); got != wallpaperSlots[1] {
		t.Errorf("with slot 0 on screen, next = %q, want %q", got, wallpaperSlots[1])
	}
	// Nothing of ours on screen, so either slot is free; slot 0 is the one
	// tried first.
	if got := nextSlot("/Users/me/Pictures/mine.jpeg", dir, ".jpeg"); got != wallpaperSlots[0] {
		t.Errorf("with a foreign wallpaper on screen, next = %q, want %q", got, wallpaperSlots[0])
	}
	if got := nextSlot("", dir, ".jpeg"); got != wallpaperSlots[0] {
		t.Errorf("with nothing known, next = %q, want %q", got, wallpaperSlots[0])
	}
}

// TestHumanBytes covers the boundaries, since the cleanup command's headline
// number is the only thing most users will read.
func TestHumanBytes(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1 MiB"},
		{4 * 1024 * 1024 * 1024, "4 GiB"},
	} {
		if got := HumanBytes(tc.in); got != tc.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
