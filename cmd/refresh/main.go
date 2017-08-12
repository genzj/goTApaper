package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/channel"
)

const (
	wallpaperPath string = "./wallpaper.jpg"
)

func main() {
	var ngChannel channel.NgPoTChannelProvider

	raw, _, err := ngChannel.Download(nil, nil)
	if err != nil {
		logrus.Panic(err)
	}

	out, err := os.Create(wallpaperPath)
	if err != nil {
		logrus.Panic(err)
	}
	defer out.Close()

	_, err = raw.WriteTo(out)
	if err != nil {
		logrus.Panic(err)
	}

	var setter actor.Win32Setter
	err = setter.Set(wallpaperPath)
	if err != nil {
		logrus.Panic(err)
	}
}
