package config

import "github.com/spf13/viper"

func GetWallpaperFileName() string {
	if viper.IsSet("wallpaper-file-name") {
		return viper.GetString("wallpaper-file-name")
	} else {
		return DefaultWallpaperFileName
	}
}
