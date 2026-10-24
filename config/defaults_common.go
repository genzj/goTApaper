package config

// Fixed names and values that are not user-configurable.
//
// Per-setting defaults live in settings_schema.go, alongside each setting's
// type and valid values. The seeded channel definitions live in the database's
// initial migration, since they are user-editable rows rather than constants.
const (
	// DefaultWallpaperFileName specifies default wallpaper downloading path
	DefaultWallpaperFileName = "wallpaper"

	// DefaultHistoryFileName specifies default name of the history file
	DefaultHistoryFileName = "history.json"

	// DefaultDBFileName specifies the name of the SQLite configuration database
	DefaultDBFileName = "gotapaper.db"

	// DefaultLogFileName specifies the default rotating log file, relative to
	// the application directory. Kept in its own subdirectory because
	// lumberjack writes its rotated siblings alongside it, and a handful of
	// timestamped files loose in the app dir would obscure the database and
	// the handshake files.
	DefaultLogFileName = "logs/gotapaper.log"

	// DefaultThumbnailDirName holds the wallpaper history thumbnails.
	DefaultThumbnailDirName = "thumbs"

	// DefaultDaemonInterval specifies default daemon downloading interval
	DefaultDaemonInterval = 3600

	// DefaultHistoryMaxEntries is how many past wallpapers the browsable
	// history keeps. An entry count rather than an age or a byte budget: at
	// roughly 40 KiB per thumbnail this bounds the directory to about 20 MiB,
	// and one rule deciding when an entry disappears is easier to reason about
	// than two that can disagree.
	DefaultHistoryMaxEntries = 500

	// DefaultThumbnailWidth is the width thumbnails are scaled down to. Wide
	// enough to recognise a photograph in a list, small enough that the whole
	// history stays cheap to keep.
	DefaultThumbnailWidth = 480
)
