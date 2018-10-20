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
	refreshCmd.PersistentFlags().StringVar(&setterName, "setter", config.DefaultSetter, "setter to configure desktop wallpaper")
	viper.BindPFlag("Setter", refreshCmd.PersistentFlags().Lookup("setter"))
	RootCmd.AddCommand(refreshCmd)
}

func refresh() {
	// reread config, in case refresh is called by daemon after a long sleep
	// during which user updated the config file
	if viper.ConfigFileUsed() == "" {
		logrus.Debugln("Using default config file")
	} else if err := viper.ReadInConfig(); err == nil {
		logrus.Debugln("Using config file:", viper.ConfigFileUsed())
	} else {
		logrus.WithFields(logrus.Fields{
			"CfgFile": viper.ConfigFileUsed(),
		}).Fatalf("Config File is not readable: %s", err)
	}

	wallpaperPath := config.GetWallpaperFileName()
	channels := viper.GetStringSlice("channels")

	v, ok := actor.Setters.Get(setterName)
	if !ok {
		logrus.Panicf("setter \"%s\" not registered", setterName)
	}
	setter := v.(actor.Setter)

	if len(channels) == 0 {
		logrus.Warnf("no channels found in the configuration file %s", viper.ConfigFileUsed())
	}

	for _, name := range channels {
		raw, _, format, err := channel.Channels.Run(name)
		if err != nil {
			logrus.Error(err)
			continue
		}

		if raw == nil {
			logrus.WithField("channel", name).Infoln("no image downloaded")
			continue
		}

		wallpaperFileName := wallpaperPath + "." + format

		out, err := os.Create(wallpaperFileName)
		if err != nil {
			logrus.Error(err)
			continue
		}
		defer out.Close()

		_, err = raw.WriteTo(out)
		if err != nil {
			logrus.Error(err)
			continue
		}

		err = setter.Set(wallpaperFileName)
		if err != nil {
			logrus.Error(err)
			continue
		}

		// exit on first success. following channels will be detected on next schedule with help of the history mechanism
		break
	}
}
