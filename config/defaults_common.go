package config

import (
	"github.com/spf13/viper"
)

const (
	// DefaultWallpaperFileName specifies default wallpaper downloading path
	DefaultWallpaperFileName = "wallpaper"

	// DefaultHistoryFileName specifies default name of the history file
	DefaultHistoryFileName = "history.json"

	// DefaultDaemonInterval specifies default daemon downloading interval
	DefaultDaemonInterval = 3600
)

// InitDefaultConfig creates default configuration
func InitDefaultConfig() {
	viper.SetDefault("language", "en-us")
	viper.SetDefault("debug", false)
	viper.SetDefault("proxy", "direct")
	viper.SetDefault("daemon.interval", 3600)
	viper.SetDefault("channels", []string{"ng-photo-of-today", "bing-wallpaper"})
	viper.SetDefault("setter", DefaultSetter)
	viper.SetDefault("ng-photo-of-today.strategy", "largest")
	viper.SetDefault("bing-wallpaper.strategy", "largest-no-logo")
}
