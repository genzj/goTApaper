//go:build darwin
// +build darwin

package setter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"howett.net/plist"

	"github.com/genzj/goTApaper/util"
)

// The macOS wallpaper agent treats every `set picture` request as "the user
// added a new photo". It therefore keeps, per distinct image path it has ever
// been handed:
//
//   - an entry in ChoiceRequests.ImageFiles in the image extension's
//     preferences, which is the "Your Photos" row of System Settings; and
//   - a full-size copy of the file in that extension's own Caches directory,
//     referenced by the entry's copyURL.
//
// Neither is ever pruned. Separately the agent keeps rendered bitmaps of what
// it has displayed, one per request, which are pure derived data.
//
// goTApaper hands the agent a new path on every refresh (see darwinsetter.go
// for why that is unavoidable), so without help these three stores grow
// without bound -- measured at ~4 GiB after roughly two thousand refreshes.
// This file reclaims that space.
const (
	// imageExtensionBundle is the container holding the "Your Photos" index
	// and the extension's copies of every image ever set.
	imageExtensionBundle = "com.apple.wallpaper.extension.image"

	// agentBundle is the container holding the rendered bitmaps.
	agentBundle = "com.apple.wallpaper.agent"

	// choiceRequestsKey is the preferences key holding the "Your Photos" list.
	choiceRequestsKey = "ChoiceRequests.ImageFiles"

	// appleScriptBundle is the originating bundle recorded for a request that
	// arrived over AppleScript, which is how goTApaper sets the wallpaper.
	appleScriptBundle = "com.apple.systemevents"

	// agentProcess renders the desktop. Restarting it blinks the wallpaper
	// black while it redraws, so it is only ever done on explicit request.
	//
	// Restarting WallpaperImageExtension has the same visible cost, for the
	// same reason -- the agent redraws when its content provider returns --
	// which is why nothing here does it.
	agentProcess = "WallpaperAgent"
)

// PruneOptions selects what PruneWallpaperCaches removes.
type PruneOptions struct {
	// AppDir is goTApaper's application directory. An index entry whose
	// originalURL points inside it was created by us.
	AppDir string

	// AllPhotos also removes entries goTApaper did not create, including
	// photos the user added by hand in System Settings. Those cannot be
	// recovered, so a caller should confirm with the user first.
	AllPhotos bool

	// RestartAgent restarts the wallpaper agent once the stores have been
	// rewritten, which briefly blanks the desktop. Only needed to make an
	// already-open System Settings pane drop its stale tiles.
	RestartAgent bool

	// DryRun reports what would be removed and changes nothing. No process is
	// restarted either, so a dry run cannot disturb the running desktop.
	DryRun bool

	// KeepPaths are image paths whose entries, copies and renders must
	// survive regardless of anything else.
	//
	// This exists because asking the system for the current wallpaper is not
	// enough for a caller that has just changed it: `set picture` returns
	// before the agent has switched, so a prune running straight afterwards
	// still sees the previous path as current and would treat the wallpaper
	// just applied as stale. A caller that knows what it set says so here.
	KeepPaths []string
}

// PruneEntry is one "Your Photos" index entry slated for removal.
type PruneEntry struct {
	// OriginalURL is the image the user (or goTApaper) asked for.
	OriginalURL string

	// CopyURL is the extension's private copy of that image, which is what
	// actually occupies the disk.
	CopyURL string

	// Bundle is the originatingBundleIdentifier the agent recorded.
	Bundle string

	// Owned reports whether OriginalURL lives in goTApaper's app directory.
	Owned bool

	// CopyBytes is the size of CopyURL on disk, or 0 if it is already gone.
	CopyBytes int64

	// Renders are the agent's rendered bitmaps for this entry's image, newest
	// last. There is one per time the image was applied, so an entry commonly
	// has several.
	Renders []string

	// RenderBytes is the total size of Renders.
	RenderBytes int64
}

// PruneStats is the itemised plan, whether or not it was applied.
type PruneStats struct {
	// Entries are the index entries to drop, with their copies.
	Entries []PruneEntry

	// Renders are the rendered bitmap files to drop.
	Renders []string

	// RenderBytes is the total size of Renders.
	RenderBytes int64

	// OrphanRenders are bitmaps in Renders that match no surviving index
	// entry, so there is nothing left to attribute them to. Listed separately
	// because they are the one group removed on inference rather than on a
	// name match.
	OrphanRenders []string

	// OrphanBytes is the total size of OrphanRenders.
	OrphanBytes int64

	// OrphanCopies are files in the extension's cache that no surviving entry
	// references. Nothing can reach them again, and no entry's removal will
	// ever account for them, so they would otherwise sit there forever.
	//
	// Only populated under AllPhotos: a copy carries no record of the image it
	// was made from, so one of ours cannot be told from another app's.
	OrphanCopies []string

	// OrphanCopyBytes is the total size of OrphanCopies.
	OrphanCopyBytes int64

	// RendersKept counts rendered bitmaps left in place, because they belong
	// to an entry that survives -- including the wallpaper on screen.
	// Reported separately so a plan of "0 to remove" is distinguishable from
	// "there are none": with a single wallpaper in play the one bitmap present
	// is the current render, and keeping it is the point.
	RendersKept int

	// Kept counts index entries left alone because they are not ours.
	Kept int

	// KeptCurrent counts entries that *are* ours (or were selected by
	// AllPhotos) but were held back because they describe the wallpaper on
	// screen. Separate from Kept: reporting these as "not created by
	// goTApaper" would be untrue.
	KeptCurrent int

	// TotalBytes is everything reclaimed, copies and renders together.
	TotalBytes int64

	// DryRun reports whether the plan was applied or only computed.
	DryRun bool
}

// PruneWallpaperCaches reclaims the space macOS has accumulated on
// goTApaper's behalf. Each step is independent and best-effort: a failure is
// logged and the remaining steps still run, so a partial success still frees
// what it can.
func PruneWallpaperCaches(opts PruneOptions) (PruneStats, error) {
	stats := PruneStats{DryRun: opts.DryRun}

	// Noted before the index is read, so the orphan sweep can tell a file that
	// predates this snapshot from one the extension wrote after it. See
	// planOrphanCopies.
	snapshot := time.Now()

	entries, decoded, err := readChoiceRequests(opts.AppDir)

	// Attribution needs an index with something in it. An empty one is not
	// evidence that every bitmap is an orphan -- it is far more likely that
	// the preferences could not be read as expected -- and acting on it would
	// delete the render of the wallpaper on screen.
	canAttribute := err == nil && len(entries) > 0
	if err != nil {
		// Not fatal. The renders are the bulk of the disk use and can still
		// go, although without the index they can only be pruned by age.
		logrus.WithError(err).Warn("cannot read the wallpaper picker index")
	}

	// The entry for the wallpaper on screen is never dropped, even under
	// AllPhotos. Nothing observed suggests the renderer reads the extension's
	// copy rather than the original file, but the cost of keeping one entry is
	// a single row in a picker, and the cost of being wrong is a blank
	// desktop.
	protected := protectedPaths(opts.KeepPaths)

	keep := make([][]byte, 0, len(entries))
	for i, raw := range entries {
		e := decoded[i]
		if path, ok := localPath(e.OriginalURL); ok && protected[path] {
			logrus.Debugf("keeping %s: it is the current wallpaper", path)
			keep = append(keep, raw)
			stats.KeptCurrent++
			continue
		}
		if opts.AllPhotos || e.Owned {
			stats.Entries = append(stats.Entries, e)
			continue
		}
		keep = append(keep, raw)
		stats.Kept++
	}

	// Renders are attributed only after the entries are classified, because a
	// bitmap's name identifies the image it renders, not the entry: its
	// prefix is sha256 of the original file's path. That lets each bitmap be
	// matched to the entry being dropped, instead of guessing from age.
	planRendersForEntries(&stats, keepPrefixes(decoded, stats.Entries), canAttribute)

	// Orphan copies are only swept under AllPhotos. A copy file is a UUID with
	// no record of where it came from, so unlike an entry or a bitmap there is
	// nothing in it to attribute: an orphan left behind by some other app's
	// wallpaper is indistinguishable from one of ours. Removing those is a
	// decision for the user to confirm, not something to do behind their back
	// on every refresh.
	if canAttribute && opts.AllPhotos {
		planOrphanCopies(&stats, decoded, snapshot)
	}

	for _, e := range stats.Entries {
		stats.TotalBytes += e.CopyBytes
	}
	stats.TotalBytes += stats.RenderBytes + stats.OrphanCopyBytes

	if opts.DryRun {
		return stats, nil
	}

	// The image extension is deliberately left running.
	//
	// An earlier revision restarted it first, on the theory that it held the
	// index in memory and would write the old copy back. It does not: a
	// dropped entry stays dropped with the extension alive and adding further
	// entries of its own, because it reads the index through cfprefsd, which
	// `defaults import` updates underneath it.
	//
	// Restarting it is also not free, which is the real reason not to. The
	// agent re-renders the desktop when its content provider reappears, and
	// that redraw blinks the wallpaper black -- for a prune that leaves the
	// displayed wallpaper completely unchanged.
	if len(stats.Entries) > 0 {
		if err := writeChoiceRequests(keep); err != nil {
			logrus.WithError(err).Warn("cannot rewrite the wallpaper picker index")
		} else {
			removeCopies(stats.Entries)
		}
	}

	removeRenders(stats.Renders)
	removeOrphanCopies(stats.OrphanCopies)

	if opts.RestartAgent {
		killProcess(agentProcess)
	}

	return stats, nil
}

// protectedPaths is the set of image paths that must not be pruned: the ones
// the caller named, plus whatever the system reports as the current wallpaper.
//
// Both, not either. The caller's list covers the case where the system has not
// caught up yet; asking the system covers a wallpaper set by something else
// entirely, which no caller could have known to name.
func protectedPaths(keep []string) map[string]bool {
	protected := map[string]bool{}

	add := func(path string) {
		if path == "" {
			return
		}
		if abs, err := filepath.Abs(path); err == nil {
			protected[abs] = true
		}
	}

	for _, path := range keep {
		add(path)
	}

	// Every desktop's wallpaper, not just one: with several displays they need
	// not agree, and each one on screen has to survive.
	current, err := currentWallpapers()
	if err != nil {
		// Not fatal when the caller named a path; it just means the only
		// protection is the one it named.
		logrus.WithError(err).Debug("cannot read the current wallpaper paths")
	}
	for _, path := range current {
		add(path)
	}

	return protected
}

// decodeChoiceRequest turns one index element into a PruneEntry. Each element
// is itself a binary plist, nested inside the array in the outer one.
func decodeChoiceRequest(raw []byte, appDir string) PruneEntry {
	var req struct {
		OriginalURL struct {
			Relative string `plist:"relative"`
		} `plist:"originalURL"`
		CopyURL struct {
			Relative string `plist:"relative"`
		} `plist:"copyURL"`
		Bundle string `plist:"originatingBundleIdentifier"`
	}

	entry := PruneEntry{}
	if _, err := plist.Unmarshal(raw, &req); err != nil {
		// An entry we cannot read is an entry we must not delete.
		logrus.WithError(err).Debug("skipping an unreadable wallpaper picker entry")
		return entry
	}

	entry.OriginalURL = req.OriginalURL.Relative
	entry.CopyURL = req.CopyURL.Relative
	entry.Bundle = req.Bundle
	entry.Owned = isOwnedEntry(entry, appDir)

	if path, ok := localPath(entry.CopyURL); ok {
		if info, err := os.Stat(path); err == nil {
			entry.CopyBytes = info.Size()
		}
	}
	return entry
}

// isOwnedEntry reports whether goTApaper created this entry.
//
// The bundle identifier alone is not enough: every AppleScript-driven
// wallpaper tool reports com.apple.systemevents, so matching on it would also
// claim another tool's entries.
//
// Two further signals narrow it to us, either one sufficient. Living in the
// current application directory is the obvious one. The filename prefix
// catches the rest: --app-dir and $GOTAPAPER_APP_DIR mean the directory a
// given entry was written from may no longer be the one in use, and those
// entries are still ours to clean up -- without the prefix test they would
// accumulate untouched, which is exactly the leak this file exists to fix.
func isOwnedEntry(e PruneEntry, appDir string) bool {
	if e.Bundle != appleScriptBundle {
		return false
	}

	path, ok := localPath(e.OriginalURL)
	if !ok {
		return false
	}

	if appDir != "" && util.IsPathUnder(path, appDir) {
		return true
	}
	return strings.HasPrefix(filepath.Base(path), wallpaperSlotPrefix)
}

// localPath turns a file:// URL into a filesystem path.
func localPath(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return "", false
	}
	return filepath.Clean(u.Path), true
}

// readChoiceRequests returns the raw index elements alongside their decoded
// form, so a caller can filter on the decoded view and still write back the
// elements it keeps byte for byte. Re-encoding an element we do not understand
// risks losing fields macOS cares about.
func readChoiceRequests(appDir string) ([][]byte, []PruneEntry, error) {
	prefs, err := exportPreferences()
	if err != nil {
		return nil, nil, err
	}

	raw, ok := prefs[choiceRequestsKey]
	if !ok {
		return nil, nil, nil
	}

	elements, ok := raw.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("%s is %T, expected an array", choiceRequestsKey, raw)
	}

	entries := make([][]byte, 0, len(elements))
	decoded := make([]PruneEntry, 0, len(elements))
	for _, el := range elements {
		b, ok := el.([]byte)
		if !ok {
			continue
		}
		entries = append(entries, b)
		decoded = append(decoded, decodeChoiceRequest(b, appDir))
	}
	return entries, decoded, nil
}

// exportPreferences reads the extension's preferences through `defaults`
// rather than off disk. The file is owned by cfprefsd, which caches it and
// would overwrite a direct write with the copy it is still holding.
func exportPreferences() (map[string]interface{}, error) {
	out, err := exec.Command("defaults", "export", imageExtensionPrefsPath(), "-").Output()
	if err != nil {
		return nil, fmt.Errorf("defaults export failed: %w", err)
	}

	prefs := map[string]interface{}{}
	if _, err := plist.Unmarshal(out, &prefs); err != nil {
		return nil, fmt.Errorf("cannot decode the exported preferences: %w", err)
	}
	return prefs, nil
}

// writeChoiceRequests replaces the index with the entries to keep, going back
// through `defaults` for the same reason exportPreferences does.
func writeChoiceRequests(keep [][]byte) error {
	prefs, err := exportPreferences()
	if err != nil {
		return err
	}

	// An empty slice rather than nil, so the key survives as an empty array
	// instead of encoding as absent.
	elements := make([]interface{}, 0, len(keep))
	for _, b := range keep {
		elements = append(elements, b)
	}
	prefs[choiceRequestsKey] = elements

	encoded, err := plist.Marshal(prefs, plist.XMLFormat)
	if err != nil {
		return fmt.Errorf("cannot encode the preferences: %w", err)
	}

	cmd := exec.Command("defaults", "import", imageExtensionPrefsPath(), "-")
	cmd.Stdin = bytes.NewReader(encoded)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("defaults import failed: %w: %s", err, out)
	}

	logrus.Debugf("wallpaper picker index rewritten with %d entries", len(keep))
	return nil
}

// planOrphanCopies finds files in the extension's cache that no entry
// references.
//
// They arise whenever an entry goes away without its copy going with it -- a
// macOS cleanup that misses one, or an interrupted prune. Nothing can display
// them and no future entry removal will account for them, so without this
// they are a permanent leak.
//
// Only files that predate the index snapshot are considered. The extension
// writes a copy asynchronously when a wallpaper is applied, so a file newer
// than the snapshot may well belong to an entry that simply was not in the
// index yet; treating it as an orphan would delete the copy of the wallpaper
// being applied right now.
func planOrphanCopies(stats *PruneStats, decoded []PruneEntry, snapshot time.Time) {
	referenced := map[string]bool{}
	for _, e := range decoded {
		if path, ok := localPath(e.CopyURL); ok {
			referenced[path] = true
		}
	}

	matches, err := filepath.Glob(filepath.Join(imageExtensionCacheDir(), "*"))
	if err != nil {
		return
	}

	// Entries being dropped already account for their own copies, so they
	// must not also be counted here.
	dropping := map[string]bool{}
	for _, e := range stats.Entries {
		if path, ok := localPath(e.CopyURL); ok {
			dropping[path] = true
		}
	}

	sort.Strings(matches)
	for _, path := range matches {
		if referenced[path] || dropping[path] {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || !info.ModTime().Before(snapshot) {
			continue
		}
		stats.OrphanCopies = append(stats.OrphanCopies, path)
		stats.OrphanCopyBytes += info.Size()
	}
}

// removeOrphanCopies deletes the unreferenced files planOrphanCopies found.
// They are already absolute paths from inside the cache directory, but the
// containment check is repeated rather than assumed.
func removeOrphanCopies(paths []string) {
	cacheDir := imageExtensionCacheDir()
	for _, path := range paths {
		if !util.IsPathUnder(path, cacheDir) {
			logrus.Warnf("refusing to remove %s: outside the wallpaper cache", path)
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logrus.WithError(err).Warnf("cannot remove the orphaned copy %s", path)
		}
	}
}

// removeCopies deletes the extension's private copies of the dropped entries.
func removeCopies(entries []PruneEntry) {
	cacheDir := imageExtensionCacheDir()
	for _, e := range entries {
		path, ok := localPath(e.CopyURL)
		if !ok {
			continue
		}
		// The copyURL comes out of Apple's own state, but it decides what gets
		// deleted, so it is confined to the directory it is supposed to name.
		if !util.IsPathUnder(path, cacheDir) {
			logrus.Warnf("refusing to remove %s: outside the wallpaper cache", path)
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logrus.WithError(err).Warnf("cannot remove the cached copy %s", path)
		}
	}
}

// renderPrefix is the agent's name for an image's renders.
//
// A rendered bitmap is named
//
//	<sha256(original path)>-<width>-<height>-<scale>-<set time>.bmp
//
// where the set time is a CFAbsoluteTime as a big-endian float64 in hex.
// Verified by setting a known path and predicting the resulting filename.
// Only the prefix is needed here: the rest varies per display and per time
// the image was applied, which is why one image has several renders.
func renderPrefix(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

// renderPrefixOf extracts the prefix from a bitmap's filename.
func renderPrefixOf(path string) string {
	base := filepath.Base(path)
	if i := strings.IndexByte(base, '-'); i > 0 {
		return base[:i]
	}
	return ""
}

// keepPrefixes collects the render prefixes of every entry that survives, so
// their bitmaps can be recognised and left alone. The current wallpaper is
// always among them, which is what makes keeping its render exact rather than
// a guess about which file is newest.
func keepPrefixes(decoded, dropped []PruneEntry) map[string]bool {
	isDropped := make(map[string]bool, len(dropped))
	for _, e := range dropped {
		isDropped[e.OriginalURL] = true
	}

	prefixes := map[string]bool{}
	for _, e := range decoded {
		if isDropped[e.OriginalURL] {
			continue
		}
		if path, ok := localPath(e.OriginalURL); ok {
			prefixes[renderPrefix(path)] = true
		}
	}
	return prefixes
}

// planRendersForEntries attributes each rendered bitmap to the entry it
// renders and records those that can go on stats.
//
// Three outcomes per bitmap: it belongs to an entry being dropped, so it goes
// with it; it belongs to a surviving entry, so it stays; or it matches no
// entry at all, meaning whatever it rendered is no longer tracked by the
// system and nothing can display it again, so it goes as an orphan.
//
// When canAttribute is false there is no attribution to be had, and it falls
// back to keeping only the newest bitmap -- the old behaviour, which is the
// most that can be said safely without knowing what belongs to whom.
func planRendersForEntries(stats *PruneStats, keep map[string]bool, canAttribute bool) {
	matches, err := filepath.Glob(filepath.Join(agentRenderDir(), "*.bmp"))
	if err != nil || len(matches) == 0 {
		return
	}

	sizes := make(map[string]int64, len(matches))
	newest, newestTime := "", int64(-1)
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		sizes[path] = info.Size()
		if mod := info.ModTime().UnixNano(); mod > newestTime {
			newest, newestTime = path, mod
		}
	}

	// Index the entries being dropped by prefix, so a bitmap can be filed
	// under the one it renders. Pointers, because the entries are updated in
	// place as their bitmaps are found.
	byPrefix := map[string][]*PruneEntry{}
	for i := range stats.Entries {
		if path, ok := localPath(stats.Entries[i].OriginalURL); ok {
			prefix := renderPrefix(path)
			byPrefix[prefix] = append(byPrefix[prefix], &stats.Entries[i])
		}
	}

	sort.Strings(matches)
	for _, path := range matches {
		size, readable := sizes[path]
		if !readable {
			// Left alone rather than deleted unexamined.
			stats.RendersKept++
			continue
		}

		if !canAttribute {
			if path == newest {
				stats.RendersKept++
				continue
			}
			stats.Renders = append(stats.Renders, path)
			stats.RenderBytes += size
			continue
		}

		prefix := renderPrefixOf(path)
		if keep[prefix] {
			stats.RendersKept++
			continue
		}

		stats.Renders = append(stats.Renders, path)
		stats.RenderBytes += size

		owners := byPrefix[prefix]
		if len(owners) == 0 {
			stats.OrphanRenders = append(stats.OrphanRenders, path)
			stats.OrphanBytes += size
			continue
		}
		for _, e := range owners {
			e.Renders = append(e.Renders, path)
			e.RenderBytes += size
		}
	}
}

func removeRenders(renders []string) {
	if len(renders) == 0 {
		return
	}

	drop := make(map[string]bool, len(renders))
	for _, path := range renders {
		drop[path] = true
	}

	// util.RemoveFilesByGlob re-globs rather than taking the planned list, so
	// a bitmap written between planning and here is left alone instead of
	// being deleted unexamined.
	if err := util.RemoveFilesByGlob(
		filepath.Join(agentRenderDir(), "*.bmp"),
		func(path string) bool { return drop[path] },
	); err != nil {
		logrus.WithError(err).Warn("cannot remove the rendered wallpaper bitmaps")
	}
}

// killProcess restarts one of the wallpaper daemons. Both are launchd jobs
// that come back on demand, and neither being absent is an error worth
// reporting -- it just means it was not running.
func killProcess(name string) {
	logrus.Debugf("restarting %s", name)
	if err := exec.Command("killall", name).Run(); err != nil {
		logrus.WithError(err).Debugf("%s was not running", name)
	}
}

// Paths into Apple's containers. Resolved through the home directory rather
// than hardcoded, and kept in one place so the test can redirect them.
var homeDir = os.UserHomeDir

func containerDir(bundle string) string {
	home, err := homeDir()
	if err != nil {
		logrus.WithError(err).Warn("cannot determine the home directory")
		return ""
	}
	return filepath.Join(home, "Library", "Containers", bundle, "Data", "Library")
}

func imageExtensionPrefsPath() string {
	return filepath.Join(
		containerDir(imageExtensionBundle), "Preferences", imageExtensionBundle+".plist",
	)
}

func imageExtensionCacheDir() string {
	return filepath.Join(containerDir(imageExtensionBundle), "Caches")
}

func agentRenderDir() string {
	return filepath.Join(
		containerDir(agentBundle), "Caches", "com.apple.wallpaper.caches",
		"extension-"+imageExtensionBundle,
	)
}

// DescribeEntry renders one entry as a single line for the cleanup command's
// listing: what was asked for, and the copy that is actually occupying disk.
func DescribeEntry(e PruneEntry) string {
	return fmt.Sprintf(
		"%s -> %s (%s)",
		describePrunePath(e.OriginalURL), trimmed(e.CopyURL), HumanBytes(e.CopyBytes),
	)
}

// describePrunePath shortens a path for display, so a listing of a few
// thousand entries stays readable.
func describePrunePath(raw string) string {
	path, ok := localPath(raw)
	if !ok {
		return raw
	}
	if home, err := homeDir(); err == nil && util.IsPathUnder(path, home) {
		if rel, err := filepath.Rel(home, path); err == nil {
			return "~" + string(filepath.Separator) + rel
		}
	}
	return path
}

// trimmed keeps a long cache filename from dominating a listing line.
func trimmed(raw string) string {
	path, ok := localPath(raw)
	if !ok {
		return raw
	}
	base := filepath.Base(path)
	if len(base) > 20 {
		base = base[:12] + "…" + filepath.Ext(base)
	}
	return filepath.Join("…", filepath.Base(filepath.Dir(path)), base)
}

// HumanBytes formats a byte count for the cleanup command's output.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0") + " " + suffix
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
