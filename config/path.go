package config

import (
	"os"
	"path"

	"github.com/sirupsen/logrus"

	homedir "github.com/mitchellh/go-homedir"
)

// Deprecated: the pre-database spellings of two setting keys.
//
// They are kept only because they are exported, and are now aliases for the
// current keys. Reading a setting by these names looked up a key that is no
// longer in the schema, so GetString returned "" and the user's configured
// wallpaper path was silently ignored in favour of the default. Use
// KeyWallpaperFileName and KeyHistoryFile.
const (
	// WallpaperFileSettingName in config file
	WallpaperFileSettingName = KeyWallpaperFileName
	// HistoryFileSettingName in config file
	HistoryFileSettingName = KeyHistoryFile
)

func loadAppFileName(configKey, defaultValue string) string {
	if filename := GetString(configKey); filename != "" {
		return MustExpand(filename)
	}
	p := path.Join(AppDir(), defaultValue)
	logrus.Debugf("use default value %s for %s", p, configKey)
	return p
}

// GetWallpaperFileName return a proper path for wallpaper picture
func GetWallpaperFileName() string {
	return loadAppFileName(KeyWallpaperFileName, DefaultWallpaperFileName)
}

// GetHistoryFileName return a proper path for history storage
func GetHistoryFileName() string {
	return loadAppFileName(KeyHistoryFile, DefaultHistoryFileName)
}

// GetLogFileName returns the path of the rotating log file.
func GetLogFileName() string {
	return loadAppFileName(KeyLogFile, DefaultLogFileName)
}

// GetThumbnailDir returns the directory holding wallpaper history thumbnails.
// It is not configurable: the thumbnails are an implementation detail of the
// history table, and a user who moves them would only strand the paths already
// recorded in it.
func GetThumbnailDir() string {
	return path.Join(AppDir(), DefaultThumbnailDirName)
}

// MustExpand expands file paths with '~' or aborts whole app at failure
func MustExpand(filename string) string {
	l := logrus.WithField("filename", filename)
	filename, err := homedir.Expand(filename)
	if err != nil {
		l.WithError(err).Errorf("cannot expand wallpaper path with ~, use default path")
		os.Exit(-1)
	}
	l = l.WithField("expanded", filename)
	l.Debugf("file path expanded successfully")
	return filename
}
