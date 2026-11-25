package config

import (
	"os"
	"path"
	"path/filepath"

	"github.com/sirupsen/logrus"

	homedir "github.com/mitchellh/go-homedir"
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

// GetOriginalDir returns the directory holding the last applied picture
// before it was cropped and watermarked. Not configurable, like the
// thumbnails: it is an implementation detail of the watermark editor.
func GetOriginalDir() string {
	return filepath.Join(AppDir(), DefaultOriginalDirName)
}

// GetPreviewDir returns the watermark editor's working directory.
func GetPreviewDir() string {
	return filepath.Join(AppDir(), DefaultPreviewDirName)
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
