package cmd

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/channel"
	"github.com/spf13/cobra"
)

const (
	wallpaperPath string = "./wallpaper.jpg"
)

func init() {
	RootCmd.AddCommand(refreshCmd)
}

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Trigger pic downloading and wallpaper setting",
	Long:  `Trigger pic downloading and wallpaper setting`,
	Run: func(cmd *cobra.Command, args []string) {
		refresh()
	},
}

func refresh() {
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
