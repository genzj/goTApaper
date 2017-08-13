package cmd

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	wallpaperPath := config.GetWallpaperFileName()
	channels := viper.GetStringSlice("channels")

	for _, name := range channels {

		raw, _, format, err := channel.Channels.Run(name)
		if err != nil {
			logrus.Panic(err)
		}

		wallpaperFileName := wallpaperPath + "." + format

		out, err := os.Create(wallpaperFileName)
		if err != nil {
			logrus.Panic(err)
		}
		defer out.Close()

		_, err = raw.WriteTo(out)
		if err != nil {
			logrus.Panic(err)
		}

		var setter actor.Win32Setter
		err = setter.Set(wallpaperFileName)
		if err != nil {
			logrus.Panic(err)
		}
	}
}
