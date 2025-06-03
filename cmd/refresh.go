package cmd

import (
	"fmt"
	"image/jpeg"
	"math/rand"
	"os"
	"time"

	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/actor/watermark"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	errNoAvailableChannel error = fmt.Errorf("no available channel")
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Trigger pic downloading and wallpaper setting",
	Long:  `Trigger pic downloading and wallpaper setting`,
	Run: func(cmd *cobra.Command, args []string) {
		refresh(args)
	},
}

var force bool

func init() {
	refreshCmd.PersistentFlags().String("setter", config.DefaultSetter, "setter to configure desktop wallpaper")
	viper.BindPFlag("setter", refreshCmd.PersistentFlags().Lookup("setter"))
	refreshCmd.PersistentFlags().BoolVar(&force, "force", false, "ignore history file and always download")
	RootCmd.AddCommand(refreshCmd)
	rand.Seed(time.Now().UnixNano())
}

func collectActiveChannelsWithProbability() map[string]float32 {
	ans := map[string]float32{}
	var last string

	activeChannels := viper.Get("active-channels")
	if channels, ok := activeChannels.([]interface{}); ok {
	channels_loop:
		for _, value := range channels {
			switch channel := value.(type) {
			case string:
				// probability is default to 1.0 if channel set as plain string
				ans[channel] = 1.0
				last = channel
			case map[interface{}]interface{}:
				l := logrus.WithField("definition", channel)
				if len(channel) != 1 {
					l.Warn("invalid channel definition: multiple keys")
					continue channels_loop
				}
				for k, v := range channel {
					if ks, ok := k.(string); !ok {
						logrus.Warnf("invalid channel definition: non-string key %T %#v", k, k)
						continue channels_loop
					} else if vf, ok := v.(float32); ok {
						ans[ks] = vf
						last = ks
					} else if vf, ok := v.(float64); ok {
						ans[ks] = float32(vf)
						last = ks
					} else {
						logrus.Warnf("invalid channel definition: non-float key %T %#v", v, v)
						continue channels_loop
					}
				}
			default:
				logrus.Warnf("invalid channel definition: %#v", value)
			}
		}
	} else {
		logrus.Errorf("active-channels should be defined as a list")
	}

	// probability of the last item is always one to guarantee at least one detection
	ans[last] = 1.0
	if len(ans) == 0 {
		logrus.Warnf("no channels found in the configuration file %s", viper.ConfigFileUsed())
	} else {
		logrus.Debugf("active channels with probability: %#v", ans)
	}
	return ans
}

func collectSpecifiedChannels(specifiedChannels []string) map[string]float32 {
	ans := map[string]float32{}
	for _, ch := range specifiedChannels {
		ans[ch] = 1
	}
	logrus.Debugf("specified channels in args: %#v", ans)
	return ans
}

func refresh(specifiedChannels []string) (*channel.PictureMeta, error) {
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

	activeChannels := collectSpecifiedChannels(specifiedChannels)

	if len(activeChannels) == 0 {
		activeChannels = collectActiveChannelsWithProbability()
	}

	setterName := viper.GetString("setter")
	v, ok := setter.Setters.Get(setterName)
	if !ok {
		logrus.Panicf("setter \"%s\" not registered", setterName)
	}
	setter := v.(setter.Setter)

	for name, probability := range activeChannels {
		l := logrus.WithField("channel", name)
		dice := rand.Float32()
		if probability < 1 && dice > probability {
			l.WithField("dice", dice).WithField("probability", probability).Info("skipped randomly")
			continue
		}
		setting := viper.Sub("channels." + name)
		if setting == nil {
			l.Error("cannot find channel definition")
			continue
		} else if !setting.IsSet("type") {
			l.Error("type of channel not set")
			continue
		}

		setting.Set("force", force)
		l.Debugf("setting: %#v", setting.AllSettings())

		raw, img, meta, err := channel.Channels.Run(setting.GetString("type"), setting)
		if err != nil {
			l.Error(err)
			continue
		}

		if meta != nil {
			meta.Channel = setting.GetString("type")
			meta.ChannelKey = name
			l.Debugf("picture metadata %##v", meta)
		} else {
			l.Warn("no picture metadata")
			continue
		}

		if raw == nil || img == nil {
			l.Infoln("no image downloaded")
			continue
		}

		newImg := actor.DefaultCropper.Crop(img)

		newImg, err = watermark.Render(newImg, meta)

		wallpaperFileName := wallpaperPath + "." + meta.Format

		out, err := os.Create(wallpaperFileName)
		if err != nil {
			l.Error(err)
			continue
		}
		defer out.Close()

		if newImg != nil && err == nil {
			img = newImg
			err = jpeg.Encode(
				out, img, &jpeg.Options{
					Quality: 90,
				},
			)
		} else {
			// use raw bytes to avoid picture quality loss
			_, err = raw.WriteTo(out)
		}
		if err != nil {
			l.Error(err)
			continue
		}

		logrus.Debug("setting wallpaper...")
		err = setter.Set(wallpaperFileName)
		if err != nil {
			l.Error(err)
			continue
		}

		// exit on first success. following channels will be detected on next schedule with help of the history mechanism
		return meta, err
	}
	return nil, errNoAvailableChannel
}
