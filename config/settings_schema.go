package config

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/genzj/goTApaper/logging"
)

// SettingsSchemaVersion is the version of the *content* model: the set of
// setting keys and what they mean. It is distinct from the database schema
// version that PRAGMA user_version tracks, which covers table structure.
//
// It exists so an older build can recognise a configuration written by a newer
// one and refuse it, rather than silently ignoring keys it does not know and
// operating on a half-understood configuration. Bump it whenever a key is
// renamed or removed, or when the meaning of an existing value changes.
// Purely additive changes do not need a bump: an older build already ignores
// unknown keys safely.
const SettingsSchemaVersion = 1

// registeredEnums collects enum values contributed by other packages, for
// settings whose valid set cannot be written down here.
//
// The `setter` setting is the motivating case: its valid values are whichever
// setters compiled in for this OS, which only actor/setter knows. Having this
// package import actor/setter would close a cycle, since actor/setter depends
// on util and util reads configuration. So the dependency is inverted and each
// setter contributes its own name as it registers itself.
//
// Values arrive one at a time rather than as a callback over the registry, so
// correctness does not depend on when this is read relative to the init
// functions that populate it. Go runs a package's init functions in filename
// order, and a setter whose file sorts after setter.go registers later; an
// eagerly-read snapshot would have missed it.
//
// enumGeneration increments on every contribution, so Definitions can tell
// that a cached build is stale. A counter rather than resetting a sync.Once,
// because clearing the cache from here would mean taking the definitions lock
// while holding this one -- the opposite order from Definitions, and so a
// deadlock waiting to happen.
var (
	registeredEnumMu sync.Mutex
	registeredEnums  = map[string][]string{}
	enumGeneration   int
)

// RegisterEnumValue adds one valid value for a setting. Call it from an init
// function; order does not matter.
func RegisterEnumValue(key, value string) {
	registeredEnumMu.Lock()
	defer registeredEnumMu.Unlock()

	if slices.Contains(registeredEnums[key], value) {
		return
	}
	registeredEnums[key] = append(registeredEnums[key], value)
	sort.Strings(registeredEnums[key])
	enumGeneration++
}

// currentEnumGeneration reports how many contributions have been made.
func currentEnumGeneration() int {
	registeredEnumMu.Lock()
	defer registeredEnumMu.Unlock()
	return enumGeneration
}

func enumValuesFor(key string, fallback []string) []string {
	registeredEnumMu.Lock()
	// Copied while still holding the lock. Copying after unlocking would race
	// with a concurrent RegisterEnumValue appending to the same slice.
	values := append([]string(nil), registeredEnums[key]...)
	registeredEnumMu.Unlock()

	if len(values) == 0 {
		return fallback
	}
	return values
}

// Definition is a global setting's spec. It is the same type as a channel
// option's, so there is one place to extend and one validator.
type Definition = ValueSpec

// Setting keys.
//
// The notation is `namespace.sub-namespace.key-name`: dots separate
// namespaces, dashes separate words within a name. Earlier revisions mixed the
// two -- `reference-width` alongside `api.port` -- which left no way to tell a
// namespace boundary from a word boundary.
//
// Keys are referenced from code rather than spelled as literals, so a rename
// is a compile error instead of a silent lookup miss. Renames also need a
// LegacyKeys entry below, or an existing YAML file stops importing.
const (
	KeyLanguage = "language"

	KeyLogDebug          = "log.debug"
	KeyLogDebugRendering = "log.debug-rendering"

	KeyLogFile       = "log.file"
	KeyLogMaxSizeMB  = "log.max-size-mb"
	KeyLogMaxBackups = "log.max-backups"
	KeyLogRingSize   = "log.ring-size"

	KeyNetworkProxy = "network.proxy"

	KeyPictureCrop            = "picture.crop"
	KeyPictureReferenceWidth  = "picture.reference-width"
	KeyPictureReferenceHeight = "picture.reference-height"

	KeyPictureMacOSCacheCleanup = "picture.macos-cache-cleanup"

	KeyWallpaperSetter   = "wallpaper.setter"
	KeyWallpaperFileName = "wallpaper.file-name"

	KeyDaemonInterval = "daemon.interval"

	KeyHistoryFile       = "history.file"
	KeyHistoryMaxEntries = "history.max-entries"
	KeyHistoryThumbnails = "history.thumbnails"

	KeyAPIPort = "api.port"
)

// definitions is built lazily because the enum values for the setter setting
// depend on a registry populated by init functions in another package.
// Building it at package-variable initialisation time would race with that
// registration.
var (
	definitionsMu  sync.Mutex
	definitions    []Definition
	definitionsGen = -1
)

// Definitions returns the authoritative list of global settings.
func Definitions() []Definition {
	definitionsMu.Lock()
	defer definitionsMu.Unlock()

	// Rebuild whenever another package has contributed an enum value since the
	// last build, so a setter registering after this was first read is still
	// reflected.
	if gen := currentEnumGeneration(); definitions == nil || definitionsGen != gen {
		definitionsGen = gen
		definitions = buildDefinitions()
	}
	return definitions
}

func buildDefinitions() []Definition {
	return []Definition{
		{
			Key: KeyLanguage, Kind: KindString, Default: "en-us",
			Label:       "Language",
			Description: "language used for application output, in xx-YY format",
		},
		{
			Key: KeyLogDebug, Kind: KindBool, Default: "false",
			Label:       "Debug logging",
			Description: "enable verbose debug log",
			LegacyKeys:  []string{"debug"},
		},
		{
			Key: KeyLogDebugRendering, Kind: KindBool, Default: "false",
			Label:       "Watermark rendering debug",
			Description: "enable debug mode in watermark rendering",
			LegacyKeys:  []string{"debug-rendering"},
		},
		{
			Key: KeyLogFile, Kind: KindString, Default: "",
			Label: "Log file",
			Description: "path of the rotating log file; empty uses " +
				DefaultLogFileName + " in the application directory",
		},
		{
			Key: KeyLogMaxSizeMB, Kind: KindInt,
			Default:     strconv.Itoa(logging.DefaultMaxSizeMB),
			Label:       "Log file size limit",
			Description: "megabytes a log file may reach before it is rotated",
		},
		{
			Key: KeyLogMaxBackups, Kind: KindInt,
			Default: strconv.Itoa(logging.DefaultMaxBackups),
			Label:   "Rotated logs kept",
			Description: "number of rotated log files to keep; with the size limit " +
				"this is the whole disk budget for logs",
		},
		{
			Key: KeyLogRingSize, Kind: KindInt,
			Default: strconv.Itoa(logging.DefaultRingSize),
			Label:   "Log entries kept in memory",
			Description: "recent entries held for reading and live tailing; " +
				"bounds memory use, as the file limits bound disk use",
		},
		{
			Key: KeyNetworkProxy, Kind: KindString, Default: "direct",
			Label:       "Proxy",
			Description: `"direct", "environment" (Linux only), or a proxy URL`,
			LegacyKeys:  []string{"proxy"},
		},
		{
			Key: KeyPictureCrop, Kind: KindEnum, Default: CropYes, EnumValues: CropValues,
			Label:       "Crop",
			Description: "crop the picture to the reference display ratio",
			LegacyKeys:  []string{"crop"},
		},
		{
			Key: KeyPictureReferenceWidth, Kind: KindInt, Default: "1920",
			Label:       "Reference width",
			Description: "reference display width, used for cropping and watermark sizing",
			LegacyKeys:  []string{"reference-width"},
		},
		{
			Key: KeyPictureReferenceHeight, Kind: KindInt, Default: "1080",
			Label:       "Reference height",
			Description: "reference display height, used for cropping and watermark sizing",
			LegacyKeys:  []string{"reference-height"},
		},
		{
			Key: KeyPictureMacOSCacheCleanup, Kind: KindBool, Default: "true",
			Label: "macOS wallpaper cache cleanup",
			Description: "macOS only: after applying a wallpaper, remove the copies and " +
				"rendered bitmaps the system keeps for wallpapers goTApaper has replaced",
		},
		{
			Key: KeyWallpaperSetter, Kind: KindEnum, Default: DefaultSetter,
			// Supplied by actor/setter, because only it knows which setters
			// this OS compiled in.
			EnumValues:  enumValuesFor(KeyWallpaperSetter, []string{DefaultSetter}),
			Label:       "Setter",
			Description: "mechanism used to apply the wallpaper to the desktop",
			LegacyKeys:  []string{"setter"},
		},
		{
			Key: KeyWallpaperFileName, Kind: KindString, Default: "",
			Label:       "Wallpaper file",
			Description: "path and basename of the downloaded wallpaper, without extension",
			LegacyKeys:  []string{"wallpaper-file-name"},
		},
		{
			Key: KeyDaemonInterval, Kind: KindInt, Default: strconv.Itoa(DefaultDaemonInterval),
			Label:       "Refresh interval",
			Description: "seconds to sleep between two background refreshes",
		},
		{
			Key: KeyHistoryFile, Kind: KindString, Default: "",
			Label:       "Legacy history file",
			Description: "path to the legacy history file; unused once history lives in the database",
			LegacyKeys:  []string{"history-file"},
		},
		{
			Key: KeyHistoryMaxEntries, Kind: KindInt,
			Default: strconv.Itoa(DefaultHistoryMaxEntries),
			Label:   "Wallpapers remembered",
			Description: "how many past wallpapers to keep in the browsable history; " +
				"older entries and their thumbnails are removed. 0 keeps everything",
		},
		{
			Key: KeyHistoryThumbnails, Kind: KindBool, Default: "true",
			Label: "Keep thumbnails",
			Description: "save a small copy of each wallpaper so the history can be " +
				"previewed; turning this off removes the existing thumbnails",
		},
		{
			Key: KeyAPIPort, Kind: KindInt, Default: "0",
			Label:       "API port",
			Description: "port for the localhost REST API; 0 picks an ephemeral port",
		},
	}
}

// Lookup finds the definition for a key.
func Lookup(key string) (Definition, bool) {
	for _, d := range Definitions() {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}

// LookupLegacy finds the definition a legacy key maps to, for importing an
// older configuration file. It also accepts a current key, so a caller does
// not have to try both.
func LookupLegacy(key string) (Definition, bool) {
	if d, ok := Lookup(key); ok {
		return d, true
	}
	for _, d := range Definitions() {
		if slices.Contains(d.LegacyKeys, key) {
			return d, true
		}
	}
	return Definition{}, false
}

// ErrUnknownKey is returned for a key absent from Definitions.
var ErrUnknownKey = fmt.Errorf("unknown setting key")

// Validate checks a raw string value against its definition. It returns
// ErrUnknownKey for an unrecognised key and an *InvalidValueError for a value
// of the wrong type or outside an enum.
func Validate(key, value string) error {
	d, ok := Lookup(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	return d.ValidateValue(value)
}
