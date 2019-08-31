package config

import (
	"os"
	"path"

	"github.com/sirupsen/logrus"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

const (
	// WallpaperFileSettingName in config file
	WallpaperFileSettingName = "wallpaper-file-name"
	// HistoryFileSettingName in config file
	HistoryFileSettingName = "history-file"
)

func loadAppFileName(configKey, defaultValue string) string {
	if viper.IsSet(configKey) {
		filename := viper.GetString(configKey)
		if filename != "" {
			return MustExpand(filename)
		}
	}
	p := path.Join(AppDir(), defaultValue)
	logrus.Debugf("use default value %s for %s", p, configKey)
	return p
}

// GetWallpaperFileName return a proper path for wallpaper picture
func GetWallpaperFileName() string {
	return loadAppFileName(WallpaperFileSettingName, DefaultWallpaperFileName)
}

// GetHistoryFileName return a proper path for history storage
func GetHistoryFileName() string {
	return loadAppFileName(HistoryFileSettingName, DefaultHistoryFileName)
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
