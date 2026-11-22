//go:build linux

package setter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
)

// Noctalia v5 ignores wallpaper-set when the requested path matches the path it
// is already displaying (even if the file's contents changed on disk), and it
// validates that the path is an existing regular file. Alternating between two
// fixed slot files in the same directory guarantees the path always differs
// from the current one while bounding disk usage to one extra file.
const noctaliaSlotPrefix = "wallpaper_noctalia_"

var (
	noctaliaWallpaperSlots = [2]string{noctaliaSlotPrefix + "0", noctaliaSlotPrefix + "1"}

	// noctaliaRunCommand is a hook for unit tests.
	noctaliaRunCommand = runCommand

	noctaliaSlotMu   sync.Mutex
	noctaliaLastSlot string
)

// NoctaliaSetter sets the desktop wallpaper in Noctalia Shell v5 via
// `noctalia msg wallpaper-set`.
type NoctaliaSetter int

// Set copies filename into an alternating slot file and applies it across all
// monitors via `noctalia msg wallpaper-set`.
func (NoctaliaSetter) Set(filename string) error {
	absSource, err := filepath.Abs(filename)
	if err != nil {
		return err
	}

	dir := filepath.Dir(absSource)
	ext := filepath.Ext(absSource)

	current := currentNoctaliaWallpaper()
	slotPath, err := filepath.Abs(
		filepath.Join(dir, nextNoctaliaSlot(current, dir, ext)+ext),
	)
	if err != nil {
		return err
	}

	if _, err := util.CopyFile(absSource, slotPath); err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"slot":       slotPath,
		"sourceFile": absSource,
		"before":     current,
	}).Debug("setting the Noctalia wallpaper")

	out, err := noctaliaRunCommand("noctalia", "msg", "wallpaper-set", slotPath)
	if err != nil {
		if msg := strings.TrimSpace(out); msg != "" {
			return fmt.Errorf("noctalia msg wallpaper-set failed: %s: %w", msg, err)
		}
		return fmt.Errorf("noctalia msg wallpaper-set failed: %w", err)
	}

	noctaliaSlotMu.Lock()
	noctaliaLastSlot = slotPath
	noctaliaSlotMu.Unlock()

	if cleanErr := util.RemoveFilesByGlob(
		filepath.Join(dir, noctaliaSlotPrefix+"*"),
		func(path string) bool {
			absPath, _ := filepath.Abs(path)
			return absPath != slotPath
		},
	); cleanErr != nil {
		logrus.WithError(cleanErr).Warn("error cleaning up previous Noctalia wallpaper slot")
	}

	return nil
}

func currentNoctaliaWallpaper() string {
	out, err := noctaliaRunCommand("noctalia", "msg", "wallpaper-get")
	if err == nil {
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			if abs, err := filepath.Abs(trimmed); err == nil {
				return abs
			}
			return trimmed
		}
	} else {
		logrus.WithError(err).Debug("cannot read current Noctalia wallpaper path")
	}

	noctaliaSlotMu.Lock()
	defer noctaliaSlotMu.Unlock()
	return noctaliaLastSlot
}

func nextNoctaliaSlot(current, dir, ext string) string {
	for _, slot := range noctaliaWallpaperSlots {
		candidate, err := filepath.Abs(filepath.Join(dir, slot+ext))
		if err != nil {
			continue
		}
		if !samePathOrFile(candidate, current) {
			return slot
		}
	}
	return noctaliaWallpaperSlots[0]
}

func samePathOrFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	infoA, errA := os.Stat(a)
	if errA != nil {
		return false
	}
	infoB, errB := os.Stat(b)
	if errB != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

func init() {
	Setters.Register("noctalia", NoctaliaSetter(0))
}
