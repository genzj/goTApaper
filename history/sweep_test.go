package history

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// fakePruner stands in for the store, so the sweep is tested without a
// database and the two responsibilities stay visibly separate: the store
// deletes rows, the sweep deletes files.
type fakePruner struct {
	// orphanedByPrune is returned from PruneWallpaperHistory.
	orphanedByPrune []string
	// referenced is returned from ThumbnailPaths.
	referenced []string

	pruneCalls   []int
	previewCalls []int
	pruneErr     error
	pathsErr     error
}

func (f *fakePruner) PruneWallpaperHistory(keepAtMost int) ([]string, error) {
	f.pruneCalls = append(f.pruneCalls, keepAtMost)
	if f.pruneErr != nil {
		return nil, f.pruneErr
	}
	return f.orphanedByPrune, nil
}

func (f *fakePruner) WallpaperHistoryBeyond(keepAtMost int) ([]string, error) {
	f.previewCalls = append(f.previewCalls, keepAtMost)
	if f.pruneErr != nil {
		return nil, f.pruneErr
	}
	return f.orphanedByPrune, nil
}

func (f *fakePruner) ThumbnailPaths() ([]string, error) {
	return f.referenced, f.pathsErr
}

// thumbDir builds a thumbnail directory. Each name maps to how long ago the
// file was last written, which is what the orphan grace window tests against.
func thumbDir(t *testing.T, ages map[string]time.Duration) string {
	t.Helper()

	dir := t.TempDir()
	for name, age := range ages {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		stamp := time.Now().Add(-age)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("Chtimes %s: %v", name, err)
		}
	}
	return dir
}

func remaining(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func contains(names []string, want string) bool {
	return slices.Contains(names, want)
}

func TestSweepRemovesTheThumbnailsOfPrunedRows(t *testing.T) {
	dir := thumbDir(t, map[string]time.Duration{
		"kept.jpg":  0,
		"gone.jpg":  0,
		"gone2.jpg": 0,
	})

	p := &fakePruner{
		orphanedByPrune: []string{
			filepath.Join(dir, "gone.jpg"),
			filepath.Join(dir, "gone2.jpg"),
		},
		referenced: []string{filepath.Join(dir, "kept.jpg")},
	}

	result, err := Sweep(p, SweepOptions{KeepAtMost: 1, ThumbnailDir: dir})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if result.RowsRemoved != 2 {
		t.Errorf("RowsRemoved = %d, want 2", result.RowsRemoved)
	}
	if result.FilesRemoved != 2 {
		t.Errorf("FilesRemoved = %d, want 2", result.FilesRemoved)
	}
	if result.BytesReclaimed != 20 {
		t.Errorf("BytesReclaimed = %d, want 20", result.BytesReclaimed)
	}
	if got := remaining(t, dir); len(got) != 1 || got[0] != "kept.jpg" {
		t.Errorf("remaining = %v, want [kept.jpg]", got)
	}
	if len(p.pruneCalls) != 1 || p.pruneCalls[0] != 1 {
		t.Errorf("PruneWallpaperHistory calls = %v, want [1]", p.pruneCalls)
	}
}

func TestSweepReclaimsAnOrphanOutsideTheGraceWindow(t *testing.T) {
	// The case this exists for: a crash, a deleted channel, or a deleted
	// database leaves a file no row points at. Nothing else can see it.
	dir := thumbDir(t, map[string]time.Duration{
		"referenced.jpg": 48 * time.Hour,
		"orphan.jpg":     48 * time.Hour,
	})

	p := &fakePruner{referenced: []string{filepath.Join(dir, "referenced.jpg")}}

	result, err := Sweep(p, SweepOptions{ThumbnailDir: dir})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if result.FilesRemoved != 1 {
		t.Fatalf("FilesRemoved = %d, want 1", result.FilesRemoved)
	}
	names := remaining(t, dir)
	if !contains(names, "referenced.jpg") {
		t.Error("the referenced thumbnail was removed")
	}
	if contains(names, "orphan.jpg") {
		t.Error("the orphan was not reclaimed")
	}
}

func TestSweepSparesAFreshOrphanWithinTheGraceWindow(t *testing.T) {
	// A thumbnail is written moments before its row is inserted. A sweep
	// racing a refresh must not eat it.
	dir := thumbDir(t, map[string]time.Duration{"in-flight.jpg": time.Second})

	p := &fakePruner{}

	result, err := Sweep(p, SweepOptions{ThumbnailDir: dir})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if result.FilesRemoved != 0 {
		t.Fatalf("FilesRemoved = %d, want 0", result.FilesRemoved)
	}
	if !contains(remaining(t, dir), "in-flight.jpg") {
		t.Error("a thumbnail inside the grace window was reclaimed")
	}
}

func TestSweepGraceWindowBoundary(t *testing.T) {
	dir := thumbDir(t, map[string]time.Duration{
		"just-inside.jpg":  OrphanGrace - time.Minute,
		"just-outside.jpg": OrphanGrace + time.Minute,
	})

	if _, err := Sweep(&fakePruner{}, SweepOptions{ThumbnailDir: dir}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	names := remaining(t, dir)
	if !contains(names, "just-inside.jpg") {
		t.Error("a file younger than the grace window was reclaimed")
	}
	if contains(names, "just-outside.jpg") {
		t.Error("a file older than the grace window survived")
	}
}

func TestSweepPurgeRemovesEvenReferencedThumbnails(t *testing.T) {
	// Turning thumbnails off is what removes the ones already written.
	dir := thumbDir(t, map[string]time.Duration{
		"a.jpg": 48 * time.Hour,
		"b.jpg": 48 * time.Hour,
	})

	p := &fakePruner{referenced: []string{
		filepath.Join(dir, "a.jpg"),
		filepath.Join(dir, "b.jpg"),
	}}

	result, err := Sweep(p, SweepOptions{ThumbnailDir: dir, PurgeThumbnails: true})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.FilesRemoved != 2 {
		t.Fatalf("FilesRemoved = %d, want 2", result.FilesRemoved)
	}
	if got := remaining(t, dir); len(got) != 0 {
		t.Errorf("remaining = %v, want empty", got)
	}
}

func TestSweepDryRunRemovesNothingButReportsWhatItWould(t *testing.T) {
	dir := thumbDir(t, map[string]time.Duration{
		"over-cap.jpg": 0,
		"orphan.jpg":   48 * time.Hour,
	})

	p := &fakePruner{orphanedByPrune: []string{filepath.Join(dir, "over-cap.jpg")}}

	result, err := Sweep(p, SweepOptions{KeepAtMost: 1, ThumbnailDir: dir, DryRun: true})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !result.DryRun {
		t.Error("DryRun was not echoed in the result")
	}
	// A dry run that reported nothing would make the flag useless: it must
	// name the row over the cap as well as the orphan.
	if result.RowsRemoved != 1 {
		t.Errorf("RowsRemoved = %d, want 1", result.RowsRemoved)
	}
	if result.FilesRemoved != 2 || result.BytesReclaimed != 20 {
		t.Errorf("reported %d files / %d bytes, want 2 / 20",
			result.FilesRemoved, result.BytesReclaimed)
	}

	names := remaining(t, dir)
	if !contains(names, "orphan.jpg") || !contains(names, "over-cap.jpg") {
		t.Errorf("a dry run deleted a file: %v", names)
	}
	// It must read the preview, never the deleting path.
	if len(p.pruneCalls) != 0 {
		t.Errorf("a dry run called PruneWallpaperHistory: %v", p.pruneCalls)
	}
	if len(p.previewCalls) != 1 || p.previewCalls[0] != 1 {
		t.Errorf("WallpaperHistoryBeyond calls = %v, want [1]", p.previewCalls)
	}
}

func TestSweepWithNoCapStillReclaimsOrphans(t *testing.T) {
	dir := thumbDir(t, map[string]time.Duration{"orphan.jpg": 48 * time.Hour})

	result, err := Sweep(&fakePruner{}, SweepOptions{KeepAtMost: 0, ThumbnailDir: dir})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.RowsRemoved != 0 {
		t.Errorf("RowsRemoved = %d, want 0", result.RowsRemoved)
	}
	if result.FilesRemoved != 1 {
		t.Errorf("FilesRemoved = %d, want 1", result.FilesRemoved)
	}
}

func TestSweepOnAMissingThumbnailDirectoryIsNotAnError(t *testing.T) {
	_, err := Sweep(&fakePruner{}, SweepOptions{
		ThumbnailDir: filepath.Join(t.TempDir(), "absent"),
	})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
}

func TestSweepIgnoresAnAlreadyMissingThumbnail(t *testing.T) {
	dir := t.TempDir()
	p := &fakePruner{orphanedByPrune: []string{filepath.Join(dir, "never-existed.jpg")}}

	result, err := Sweep(p, SweepOptions{KeepAtMost: 1, ThumbnailDir: dir})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	// The row went, but there was no file to reclaim.
	if result.RowsRemoved != 1 {
		t.Errorf("RowsRemoved = %d, want 1", result.RowsRemoved)
	}
	if result.FilesRemoved != 0 {
		t.Errorf("FilesRemoved = %d, want 0", result.FilesRemoved)
	}
}

func TestSweepMatchesThumbnailsByNameSoAMovedAppDirDoesNotOrphanThemAll(t *testing.T) {
	dir := thumbDir(t, map[string]time.Duration{"bing-1.jpg": 48 * time.Hour})

	// The rows still record the thumbnail under its previous location.
	p := &fakePruner{referenced: []string{"/old/app/dir/thumbs/bing-1.jpg"}}

	result, err := Sweep(p, SweepOptions{ThumbnailDir: dir})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.FilesRemoved != 0 {
		t.Fatalf("FilesRemoved = %d, want 0", result.FilesRemoved)
	}
}
