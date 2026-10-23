//go:build darwin
// +build darwin

package setter

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
)

// The wallpaper is copied to one of two fixed filenames, used alternately.
//
// macOS will not re-render a wallpaper whose path it is already displaying,
// even when the file's content has changed underneath it -- verified on macOS
// 26, where overwriting the current wallpaper file and re-issuing the same
// `set picture` produces no new render at all. So the path handed to the
// system has to differ from the one currently in use.
//
// Earlier revisions got that by suffixing a timestamp, which gave a brand new
// path every refresh. That turned out to be expensive: the wallpaper agent
// treats each distinct path as a photo the user added, and keeps both an index
// entry and its own full-size copy of the file forever. Two thousand refreshes
// left roughly 4 GiB behind (see wallpapercache_darwin.go).
//
// Alternating between two fixed names keeps the path different from the
// current one -- which is all the renderer needs -- while the agent only ever
// learns about two paths. Re-setting a path it has seen before updates its
// existing entry rather than adding another, so the cost is bounded at two
// entries no matter how long the daemon runs.
const (
	// wallpaperSlotPrefix names the slot files. It doubles as an ownership
	// marker when pruning the system's caches, so an entry written by an
	// earlier run under a different --app-dir is still recognised as ours.
	// See isOwnedEntry.
	wallpaperSlotPrefix = "wallpaper_osx_"
)

// wallpaperSlots are the two names, in the order they are tried.
var wallpaperSlots = [2]string{wallpaperSlotPrefix + "0", wallpaperSlotPrefix + "1"}

type DarwinSetter int

// Set sets the desktop wallpaper to the specified file via osascript.
func (_ DarwinSetter) Set(file string) error {
	dir, filename := filepath.Split(file)
	ext := filepath.Ext(filename)

	current, err := currentWallpaper()
	if err != nil {
		// Not fatal: without knowing the current wallpaper we may pick the
		// slot already in use and the desktop will not change this once, but
		// the next refresh picks the other one.
		logrus.WithError(err).Debug("cannot read the current wallpaper path")
	}

	wallpaperPath, err := filepath.Abs(filepath.Join(dir, nextSlot(current, dir, ext)+ext))
	if err != nil {
		return err
	}

	if _, err := util.CopyFile(file, wallpaperPath); err != nil {
		return err
	}

	if _, err := runCommand(
		"osascript",
		"-e",
		`tell application "System Events" to tell every desktop to set picture to `+
			strconv.Quote(wallpaperPath),
	); err != nil {
		return err
	}

	// Drop the other slot's file. The system is no longer displaying it, and
	// leaving it would double the space goTApaper itself occupies.
	if cleanErr := util.RemoveFilesByGlob(
		filepath.Join(dir, wallpaperSlotPrefix+"*"),
		func(path string) bool {
			absPath, _ := filepath.Abs(path)
			return absPath != wallpaperPath
		},
	); cleanErr != nil {
		logrus.Warnf("error during cleaning up old wallpapers: %s", cleanErr)
	}

	pruneWallpaperCaches(wallpaperPath)
	return nil
}

// nextSlot picks whichever slot the system is not currently displaying.
func nextSlot(current, dir, ext string) string {
	for _, slot := range wallpaperSlots {
		candidate, err := filepath.Abs(filepath.Join(dir, slot+ext))
		if err != nil {
			continue
		}
		if candidate != current {
			return slot
		}
	}
	return wallpaperSlots[0]
}

// currentWallpaper asks the system which file it is displaying. Every desktop
// gets the same wallpaper, so the first line of the reply is enough.
func currentWallpaper() (string, error) {
	out, err := runCommand(
		"osascript", "-e",
		`tell application "System Events" to tell every desktop to get picture`,
	)
	if err != nil {
		return "", err
	}

	// A list of paths, one per desktop, comma-separated on a single line.
	path := out
	for i, c := range out {
		if c == ',' || c == '\n' {
			path = out[:i]
			break
		}
	}
	if path == "" {
		return "", fmt.Errorf("no current wallpaper reported")
	}
	return filepath.Abs(path)
}

// pruneWallpaperCaches reclaims the copies and rendered bitmaps macOS keeps
// for the wallpaper this refresh just replaced.
//
// It runs on every refresh. An earlier revision throttled it behind a count of
// index entries, from when a timestamped filename meant those entries grew
// without bound. Two things make that wrong now: the two slots cap the entries
// at two, so any useful threshold is never reached, while the bitmaps -- 15
// MiB each -- do still accumulate one per refresh. And the cost that motivated
// throttling is gone: nothing is restarted and the desktop is not redrawn, so
// there is nothing to spare the user from.
// justSet is the path handed to osascript a moment ago. It has to be named
// explicitly: the agent switches asynchronously, so asking it which wallpaper
// is current here still returns the previous one, and the wallpaper just
// applied would be pruned as stale.
func pruneWallpaperCaches(justSet string) {
	if !config.GetBool(config.KeyPictureMacOSCacheCleanup) {
		logrus.Debug("macOS wallpaper cache cleanup is disabled")
		return
	}

	stats, err := PruneWallpaperCaches(PruneOptions{
		AppDir:    config.AppDir(),
		KeepPaths: []string{justSet},
	})
	if err != nil {
		logrus.WithError(err).Warn("cannot prune the macOS wallpaper caches")
		return
	}

	// Silent on the common case of nothing to do, so a daemon's log is not one
	// line of "reclaimed 0 B" per refresh.
	if stats.TotalBytes == 0 {
		return
	}
	logrus.Infof(
		"reclaimed %s from the macOS wallpaper caches "+
			"(%d picker entries, %d rendered bitmaps, %d unreferenced copies)",
		HumanBytes(stats.TotalBytes),
		len(stats.Entries), len(stats.Renders), len(stats.OrphanCopies),
	)
}

func init() {
	Setters.Register("darwin", DarwinSetter(0))
}
