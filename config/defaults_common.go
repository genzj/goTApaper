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

	// DefaultDaemonInterval specifies default daemon downloading interval
	DefaultDaemonInterval = 3600
)
