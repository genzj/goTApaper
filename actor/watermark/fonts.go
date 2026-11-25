package watermark

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/font/sfnt"
)

// FontInfo describes one font the watermark renderer can load.
type FontInfo struct {
	// ID identifies the font within this process's index, for fetching its
	// file. It is derived from the path, so it is stable across rescans.
	ID string `json:"id"`
	// Family and Style are the font's own names, from its name table.
	Family string `json:"family"`
	Style  string `json:"style"`
	// File is the base file name, which is what a layer stores: it resolves
	// on another machine that has the same font installed elsewhere.
	File string `json:"file"`
	// Path is where the file is on this machine.
	Path string `json:"path"`
}

// fontIndex is every loadable font under the platform font directories.
//
// Scanned once per process, on first use, and kept for the process's
// lifetime: walking and parsing the font directories on every render is what
// made the old lookup slow, and a live preview renders many times a second.
// A font installed later appears after restarting goTApaper.
type fontIndex struct {
	once   sync.Once
	fonts  []FontInfo
	byFile map[string]FontInfo // lower-cased base name -> font
	byID   map[string]FontInfo

	// unsupported are installed fonts the renderer cannot load, kept to
	// say why rather than to offer them.
	unsupported       []UnsupportedFont
	unsupportedByFile map[string]UnsupportedFont
}

// UnsupportedFont is an installed font the renderer cannot load, and why.
type UnsupportedFont struct {
	Family string `json:"family"`
	Style  string `json:"style"`
	File   string `json:"file"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

var fonts = &fontIndex{}

// index returns the font index, building it on first use. It is read-only
// once built, so callers need no lock.
func index() *fontIndex {
	fonts.once.Do(fonts.build)
	return fonts
}

// fontDirectories is where fonts are looked for; a variable so tests can point
// it at a fixture directory instead of the machine's fonts.
var fontDirectories = getFontDirectories

// Fonts lists every font the renderer can load, sorted by family and style.
func Fonts() []FontInfo {
	x := index()
	out := make([]FontInfo, len(x.fonts))
	copy(out, x.fonts)
	return out
}

// UnsupportedFonts lists the installed fonts the renderer cannot load, with
// the reason for each, so the editor can explain why a font is missing from
// its list.
func UnsupportedFonts() []UnsupportedFont {
	x := index()
	out := make([]UnsupportedFont, len(x.unsupported))
	copy(out, x.unsupported)
	return out
}

// FontByID returns the indexed font with the given id.
func FontByID(id string) (FontInfo, bool) {
	f, ok := index().byID[id]
	return f, ok
}

// build scans the font directories. Run once, by index.
func (x *fontIndex) build() {
	started := time.Now()
	found, unsupported := scanFonts(fontDirectories())
	x.fonts = found
	x.unsupported = unsupported
	x.unsupportedByFile = make(map[string]UnsupportedFont, len(unsupported))
	for _, u := range unsupported {
		key := strings.ToLower(u.File)
		if _, dup := x.unsupportedByFile[key]; !dup {
			x.unsupportedByFile[key] = u
		}
	}
	x.byFile = make(map[string]FontInfo, len(found))
	x.byID = make(map[string]FontInfo, len(found))
	for _, f := range found {
		key := strings.ToLower(f.File)
		// The first directory wins, as it did for the old walk: user fonts
		// are listed before system ones.
		if _, dup := x.byFile[key]; !dup {
			x.byFile[key] = f
		}
		x.byID[f.ID] = f
	}
	logrus.WithFields(logrus.Fields{
		"fonts":       len(found),
		"unsupported": len(unsupported),
		"elapsed":     time.Since(started).Round(time.Millisecond).String(),
	}).Info("indexed watermark fonts")
}

// scanFonts walks dirs in order and returns every loadable font in them, and
// the fonts it found but the renderer cannot load.
func scanFonts(dirs []string) ([]FontInfo, []UnsupportedFont) {
	var out []FontInfo
	var unsupported []UnsupportedFont
	seen := map[string]bool{}
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !isFontFile(d.Name()) {
				return nil
			}
			if seen[path] {
				return nil
			}
			seen[path] = true
			f, ok := readFontInfo(path)
			if !ok {
				return nil
			}
			if problem := loadProblem(path); problem != "" {
				unsupported = append(unsupported, UnsupportedFont{
					Family: f.Family, Style: f.Style, File: f.File, Path: f.Path, Reason: problem,
				})
				return nil
			}
			out = append(out, f)
			return nil
		})
	}
	sort.SliceStable(unsupported, func(i, j int) bool {
		return strings.ToLower(unsupported[i].Family) < strings.ToLower(unsupported[j].Family)
	})
	sort.SliceStable(out, func(i, j int) bool {
		fi, fj := strings.ToLower(out[i].Family), strings.ToLower(out[j].Family)
		if fi != fj {
			return fi < fj
		}
		return styleRank(out[i].Style) < styleRank(out[j].Style)
	})
	return out, unsupported
}

// loadProblem parses a font the way the renderer will and returns why it
// cannot be loaded, or "" if it can.
//
// The header and name checks in readFontInfo are not enough: golang/freetype
// also rejects fonts using tables it does not implement -- Apple's version 1
// kern table, some cmap encodings -- and only a full parse finds those. It
// reads the whole file, once per process: on a typical machine a few hundred
// megabytes of fonts parse in well under a second.
func loadProblem(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if _, err := parseFont(data); err != nil {
		return describeFontError(err)
	}
	return ""
}

// parseFont is the renderer's parser; a variable so a test can stand in a
// font it rejects without needing such a font.
var parseFont = truetype.Parse

// describeFontError turns the parser's error into something a user can act
// on: freetype's "unsupported TrueType feature: kern version: 1" names the
// table, but not that the fix is another font.
func describeFontError(err error) string {
	msg := err.Error()
	const unsupported = "freetype: unsupported TrueType feature: "
	if feature, ok := strings.CutPrefix(msg, unsupported); ok {
		return "uses a TrueType feature the renderer cannot read (" + feature + ")"
	}
	return strings.TrimPrefix(msg, "freetype: ")
}

// styleRank orders a family's styles with the regular face first.
func styleRank(style string) string {
	switch strings.ToLower(style) {
	case "regular", "book", "normal", "roman":
		return "0"
	}
	return "1" + strings.ToLower(style)
}

// readFontInfo reads a font's names, and reports false for a file the
// renderer cannot load.
//
// Only the name table is read, through a lazy parser, so indexing a directory
// of large CJK fonts does not read them whole. The renderer's parser
// (golang/freetype) only understands TrueType outlines, so a CFF-flavoured
// OpenType file is left out even though its name table is readable.
func readFontInfo(path string) (FontInfo, bool) {
	file, err := os.Open(path)
	if err != nil {
		return FontInfo{}, false
	}
	defer file.Close()

	magic := make([]byte, 4)
	if _, err := io.ReadFull(file, magic); err != nil {
		return FontInfo{}, false
	}
	// 0x00010000 and "true" are TrueType outlines; "OTTO" is CFF and "ttcf"
	// a collection, neither of which the renderer can load.
	if string(magic) != "\x00\x01\x00\x00" && string(magic) != "true" {
		return FontInfo{}, false
	}

	f, err := sfnt.ParseReaderAt(file)
	if err != nil {
		return FontInfo{}, false
	}
	var buf sfnt.Buffer
	family := fontName(f, &buf, sfnt.NameIDTypographicFamily, sfnt.NameIDFamily)
	style := fontName(f, &buf, sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily)
	base := filepath.Base(path)
	if family == "" {
		family = stripExtension(base)
	}
	// macOS names its private interface fonts with a leading dot and hides
	// them from every font menu; so does this one.
	if strings.HasPrefix(family, ".") || strings.HasPrefix(base, ".") {
		return FontInfo{}, false
	}
	if style == "" {
		style = "Regular"
	}

	sum := sha1.Sum([]byte(path))
	return FontInfo{
		ID:     hex.EncodeToString(sum[:8]),
		Family: family,
		Style:  style,
		File:   base,
		Path:   path,
	}, true
}

// fontName returns the first of the given names the font declares.
func fontName(f *sfnt.Font, buf *sfnt.Buffer, ids ...sfnt.NameID) string {
	for _, id := range ids {
		if name, err := f.Name(buf, id); err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// lookupFont resolves a layer's font name -- a path, or a file name with or
// without its extension -- against the index. An exact file name wins; failing
// that the shortest file name containing the name, as the old lookup did.
//
// A font that is installed but cannot be loaded is reported as such, so the
// layer's error says to pick another font rather than to install this one.
func lookupFont(name string) (string, error) {
	x := index()

	needle := strings.ToLower(filepath.Base(name))
	if f, ok := x.byFile[needle]; ok {
		return f.Path, nil
	}
	if u, ok := x.unsupportedByFile[needle]; ok {
		return "", fmt.Errorf("font %s %s, so it cannot be used; pick another font", u.File, u.Reason)
	}
	notFound := fmt.Errorf("cannot find font '%s' in user or system directories", filepath.Base(name))
	needleBase := stripExtension(needle)
	if needleBase == "" {
		return "", notFound
	}
	best, bestScore := "", -1
	for _, f := range x.fonts {
		base := strings.ToLower(stripExtension(f.File))
		if !strings.Contains(base, needleBase) {
			continue
		}
		if score := len(base) - len(needleBase); bestScore < 0 || score < bestScore {
			best, bestScore = f.Path, score
		}
	}
	if best == "" {
		return "", notFound
	}
	return best, nil
}

// defaultFontPreference lists, per platform, fonts that ship with the system
// and render watermark text well, most preferred first.
var defaultFontPreference = map[string][]string{
	"darwin":  {"Helvetica", "Arial", "Verdana", "Geneva"},
	"windows": {"Segoe UI", "Arial", "Verdana", "Tahoma"},
}

// fallbackFontPreference applies on every other platform, and after a
// platform's own list.
var fallbackFontPreference = []string{
	"Noto Sans", "DejaVu Sans", "Liberation Sans", "Ubuntu", "Cantarell", "Arial",
}

// DefaultFont returns the file name of a sans-serif font that is installed
// here, for presets and new layers. It is NotoSans-Regular.ttf -- the old
// default -- when nothing better is found, which will not load but keeps the
// value familiar.
func DefaultFont() string {
	all := Fonts()
	prefs := append(append([]string{}, defaultFontPreference[runtime.GOOS]...), fallbackFontPreference...)
	for _, family := range prefs {
		for _, f := range all {
			if strings.EqualFold(f.Family, family) && styleRank(f.Style) == "0" {
				return f.File
			}
		}
	}
	for _, f := range all {
		if styleRank(f.Style) == "0" {
			return f.File
		}
	}
	return "NotoSans-Regular.ttf"
}
