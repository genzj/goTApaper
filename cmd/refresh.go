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

var setterName string

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Trigger pic downloading and wallpaper setting",
	Long:  `Trigger pic downloading and wallpaper setting`,
	Run: func(cmd *cobra.Command, args []string) {
		refresh()
	},
}

func init() {
	refreshCmd.PersistentFlags().StringVar(&setterName, "setter", DefaultSetter, "setter to configure desktop wallpaper")
	viper.BindPFlag("global.Setter", refreshCmd.PersistentFlags().Lookup("setter"))
	viper.SetDefault("global.Setter", DefaultSetter)
	RootCmd.AddCommand(refreshCmd)
}

func refresh() {
	wallpaperPath := config.GetWallpaperFileName()
	channels := viper.GetStringSlice("channels")

	v, ok := actor.Setters.Get(setterName)
	if !ok {
		logrus.Panicf("setter \"%s\" not registered", setterName)
	}
	setter := v.(actor.Setter)

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

		err = setter.Set(wallpaperFileName)
		if err != nil {
			logrus.Panic(err)
		}
	}
}
