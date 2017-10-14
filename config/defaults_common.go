package config

import (
	"github.com/spf13/viper"
)

const (
	DefaultWallpaperFileName = "./wallpaper"

	DefaultDaemonInterval = 3600
)

func InitDefaultConfig() {
	viper.SetDefault("history-file", "./history.json")
	viper.SetDefault("language", "en-us")
	viper.SetDefault("debug", false)
	viper.SetDefault("wallpaper-file-name", "wallpaper")
	viper.SetDefault("proxy", "direct")
	viper.SetDefault("daemon.interval", 3600)
	viper.SetDefault("channels", []string{"ng-photo-of-today"})
	viper.SetDefault("setter", DefaultSetter)
	viper.SetDefault("ng-photo-of-today.strategy", "largest")
}
