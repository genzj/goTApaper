package history

import (
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// OrphanGrace is how long an unreferenced thumbnail is left alone before the
// sweep reclaims it.
//
// A thumbnail is written moments before the row that references it is
// inserted, so a sweep running concurrently with a refresh would otherwise be
// free to delete a file that is about to become live. One hour is far longer
// than that window and far shorter than anyone would notice the disk use.
const OrphanGrace = time.Hour

// Pruner is the part of the store the sweep needs. A narrow interface rather
// than the whole store, so the sweep can be tested without a database and so
// it is obvious this code only ever deletes history.
type Pruner interface {
	// PruneWallpaperHistory deletes rows beyond keepAtMost and returns the
	// thumbnail paths that are now unreferenced.
	PruneWallpaperHistory(keepAtMost int) ([]string, error)
	// WallpaperHistoryBeyond returns the paths a prune would delete, without
	// deleting them.
	WallpaperHistoryBeyond(keepAtMost int) ([]string, error)
	// ThumbnailPaths returns every thumbnail path still referenced.
	ThumbnailPaths() ([]string, error)
}

// SweepOptions configures one sweep.
type SweepOptions struct {
	// KeepAtMost is the history entry cap. Zero or less keeps every row, in
	// which case only orphaned files are reclaimed.
	KeepAtMost int
	// ThumbnailDir is swept for files no row references.
	ThumbnailDir string
	// PurgeThumbnails removes every thumbnail regardless of whether a row
	// references it, for when the user turns thumbnails off.
	PurgeThumbnails bool
	// DryRun reports what would be removed without removing it.
	DryRun bool
	// Now overrides the clock, for tests. Zero means time.Now.
	Now time.Time
}

// SweepResult reports what a sweep did.
type SweepResult struct {
	// RowsRemoved is how many history entries were deleted.
	RowsRemoved int `json:"rowsRemoved"`
	// FilesRemoved counts thumbnails unlinked, both those belonging to
	// deleted rows and orphans.
	FilesRemoved int `json:"filesRemoved"`
	// BytesReclaimed is the total size of those files.
	BytesReclaimed int64 `json:"bytesReclaimed"`
	// DryRun echoes whether anything was actually removed.
	DryRun bool `json:"dryRun,omitempty"`
}

// Sweep enforces the history entry cap and reclaims thumbnail files that no
// history row references.
//
// The order matters. Rows are deleted first and their files unlinked only
// after that transaction has committed: a crash in between leaves an
// unreferenced file, which the orphan pass reclaims, whereas unlinking first
// would leave a surviving row pointing at a missing image.
//
// The orphan pass is what makes the whole thing self-correcting. It reclaims
// files left behind by a crash, by a deleted channel, by a deleted database,
// or by thumbnails being turned off -- none of which the row-driven delete
// above can see.
func Sweep(p Pruner, opts SweepOptions) (SweepResult, error) {
	result := SweepResult{DryRun: opts.DryRun}

	paths, err := prunedPaths(p, opts)
	if err != nil {
		return result, err
	}
	result.RowsRemoved = len(paths)

	removed, bytes := removeFiles(paths, opts.DryRun)
	result.FilesRemoved += removed
	result.BytesReclaimed += bytes

	orphans, err := findOrphans(p, opts)
	if err != nil {
		// The cap was already enforced, so report what succeeded rather than
		// discarding it along with the error.
		return result, err
	}
	removed, bytes = removeFiles(orphans, opts.DryRun)
	result.FilesRemoved += removed
	result.BytesReclaimed += bytes

	return result, nil
}

// prunedPaths deletes the rows beyond the cap and returns their thumbnail
// paths, or on a dry run reads the same set without deleting anything.
func prunedPaths(p Pruner, opts SweepOptions) ([]string, error) {
	if opts.DryRun {
		return p.WallpaperHistoryBeyond(opts.KeepAtMost)
	}
	return p.PruneWallpaperHistory(opts.KeepAtMost)
}

// findOrphans lists thumbnail files that no history row references.
func findOrphans(p Pruner, opts SweepOptions) ([]string, error) {
	if opts.ThumbnailDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(opts.ThumbnailDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing has been thumbnailed yet.
			return nil, nil
		}
		return nil, err
	}

	referenced := map[string]struct{}{}
	if !opts.PurgeThumbnails {
		paths, err := p.ThumbnailPaths()
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			// Compared by base name, so a thumbnail directory that moved --
			// because the app dir did -- does not make every existing row look
			// unreferenced.
			referenced[filepath.Base(path)] = struct{}{}
		}
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.Add(-OrphanGrace)

	var orphans []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := referenced[entry.Name()]; ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			// Within the grace window: this may be a thumbnail whose row is
			// about to be inserted.
			continue
		}
		orphans = append(orphans, filepath.Join(opts.ThumbnailDir, entry.Name()))
	}
	return orphans, nil
}

// removeFiles unlinks paths, returning how many went and how many bytes that
// reclaimed. A failure to remove one file is logged and skipped: a leftover
// thumbnail is clutter, not a reason to abandon the sweep.
func removeFiles(paths []string, dryRun bool) (int, int64) {
	var count int
	var bytes int64

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			// Already gone, which is the desired end state.
			continue
		}
		if dryRun {
			count++
			bytes += info.Size()
			continue
		}
		if err := os.Remove(path); err != nil {
			logrus.WithField("thumbnail", path).WithError(err).Debug(
				"cannot remove thumbnail",
			)
			continue
		}
		count++
		bytes += info.Size()
	}
	return count, bytes
}
