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
	viper.SetDefault("proxy", "direct")
	viper.SetDefault("setter", DefaultSetter)
}
