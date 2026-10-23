//go:build darwin
// +build darwin

package setter

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

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
// entry and its own full-size copy of the file forever (see
// wallpapercache_darwin.go).
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

	paths, err := currentWallpapers()
	if err != nil {
		// Not fatal: without knowing the current wallpaper we may pick the
		// slot already in use and the desktop will not change this once, but
		// the next refresh picks the other one.
		logrus.WithError(err).Debug("cannot read the current wallpaper paths")
	}

	wallpaperPath, err := filepath.Abs(
		filepath.Join(dir, nextSlot(currentSlot(paths), dir, ext)+ext),
	)
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

// currentWallpapers asks the system which file each desktop is displaying.
//
// The reply is one path per desktop, comma-separated on a single line. All of
// them are returned: the desktops usually agree, but they need not, and a
// caller deciding what is safe to delete has to account for every one.
func currentWallpapers() ([]string, error) {
	out, err := runCommand(
		"osascript", "-e",
		`tell application "System Events" to tell every desktop to get picture`,
	)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, field := range strings.Split(out, ",") {
		path := strings.TrimSpace(field)
		if path == "" {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			paths = append(paths, abs)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no current wallpaper reported")
	}
	return paths, nil
}

// currentSlot reports which of our slot files the system is displaying.
//
// With several desktops the reply can name several files, so the one that
// matters is whichever is a slot of ours -- that is the path a new wallpaper
// must differ from. Falling back to the first path keeps the comparison
// meaningful when none is ours, which is what happens on the first refresh
// after the user has set a wallpaper by hand.
func currentSlot(paths []string) string {
	for _, path := range paths {
		if strings.HasPrefix(filepath.Base(path), wallpaperSlotPrefix) {
			return path
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// pruneWallpaperCaches reclaims the copies and rendered bitmaps macOS keeps
// for the wallpaper this refresh just replaced.
//
// Running it on every refresh is deliberate, and cheap: reading the index, one
// directory listing and a handful of unlink calls, against a refresh that has
// just downloaded an image over the network. Nothing is restarted and the
// desktop is not redrawn, so there is nothing to batch or defer. It also needs
// to be this often, because the agent writes a fresh bitmap every time a
// wallpaper is applied -- one accumulates per refresh even though the two
// slots cap the index entries at two.
//
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
		"reclaimed %s from the macOS wallpaper caches (%d picker entries, %d rendered bitmaps)",
		HumanBytes(stats.TotalBytes), len(stats.Entries), len(stats.Renders),
	)
}

func init() {
	Setters.Register("darwin", DarwinSetter(0))
}
