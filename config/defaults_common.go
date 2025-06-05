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
	viper.SetDefault("active-channels", []string{"__ng-photo-of-today", "__bing-wallpaper"})
	viper.SetDefault("channels", []string{"__ng-photo-of-today", "__bing-wallpaper"})
	viper.SetDefault("channels.__ng-photo-of-today.type", "ng-photo-of-today")
	viper.SetDefault("channels.__bing-wallpaper.strategy", "largest-no-logo")
	viper.SetDefault("channels.__bing-wallpaper.type", "bing-wallpaper")
	viper.SetDefault("reference-width", 1920)
	viper.SetDefault("reference-height", 1080)
	viper.SetDefault("crop", "yes")
	viper.SetDefault("setter", DefaultSetter)
}
