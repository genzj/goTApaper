package cmd

import (
	"fmt"
	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/actor/watermark"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"image/jpeg"
	"math/rand"
	"os"
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
}

type channelsWithProbability struct {
	name string
	p    float32
}

func addNewChannel(channels []channelsWithProbability, name string, p float32) []channelsWithProbability {
	return append(channels, channelsWithProbability{
		name: name,
		p:    p,
	})
}

func collectActiveChannels() []channelsWithProbability {
	var ans []channelsWithProbability

	activeChannels := viper.Get("active-channels")
	logrus.Debugf("active channels: %#v %T", activeChannels, activeChannels)
	if channels, ok := activeChannels.([]string); ok {
		for _, ch := range channels {
			ans = addNewChannel(ans, ch, 1.0)
		}
	} else if channels, ok := activeChannels.([]interface{}); ok {
	channelsLoop:
		for _, value := range channels {
			switch ch := value.(type) {
			case string:
				// probability is default to 1.0 if channel set as plain string
				ans = addNewChannel(ans, ch, 1.0)
			case map[string]interface{}:
				l := logrus.WithField("definition", ch)
				if len(ch) != 1 {
					l.Warn("invalid channel definition: multiple keys")
					continue channelsLoop
				}
				for ks, v := range ch {
					if vf, ok := v.(float32); ok {
						ans = addNewChannel(ans, ks, vf)
					} else if vf, ok := v.(float64); ok {
						ans = addNewChannel(ans, ks, float32(vf))
					} else {
						logrus.Warnf("invalid channel definition: non-float key %T %#v", v, v)
						continue channelsLoop
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
	ans[len(ans)-1].p = 1.0
	if len(ans) == 0 {
		logrus.Warnf("no channels found in the configuration file %s", viper.ConfigFileUsed())
	} else {
		logrus.Debugf("active channels with probability: %#v", ans)
	}
	return ans
}

func collectSpecifiedChannels(specifiedChannels []string) []channelsWithProbability {
	if len(specifiedChannels) == 0 {
		return nil
	}
	var ans []channelsWithProbability
	for _, ch := range specifiedChannels {
		ans = addNewChannel(ans, ch, 1.0)
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

	activeChannels := collectSpecifiedChannels(specifiedChannels)

	if len(activeChannels) == 0 {
		activeChannels = collectActiveChannels()
	}

	setterName := viper.GetString("setter")
	v, ok := setter.Setters.Get(setterName)
	if !ok {
		logrus.Panicf("setter \"%s\" not registered", setterName)
	}
	setter := v.(setter.Setter)

	for _, ch := range activeChannels {
		name, probability := ch.name, ch.p
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

		if meta, err := detectOneChannel(name, setting, setter); err != nil || meta == nil {
			continue
		} else {
			// exit on first success. following channels will be detected on next schedule with help of the history mechanism
			return meta, err
		}

	}
	return nil, errNoAvailableChannel
}

func detectOneChannel(name string, setting *viper.Viper, setter setter.Setter) (*channel.PictureMeta, error) {
	l := logrus.WithField("channel", name)
	wallpaperPath := config.GetWallpaperFileName()

	raw, img, meta, err := channel.Channels.Run(setting.GetString("type"), setting)
	if err != nil {
		l.Error(err)
		return nil, err
	}

	if meta != nil {
		meta.Channel = setting.GetString("type")
		meta.ChannelKey = name
		l.Debugf("picture metadata %##v", meta)
	} else {
		l.Warn("no picture metadata")
		return nil, err
	}

	if raw == nil || img == nil {
		l.Infoln("no image downloaded")
		return nil, err
	}

	newImg := actor.DefaultCropper.Crop(img)

	newImg, _ = watermark.Render(newImg, meta)

	wallpaperFileName := wallpaperPath + "." + meta.Format

	out, err := os.Create(wallpaperFileName)
	if err != nil {
		l.Error(err)
		return nil, err
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			logrus.Warnf("error when closing file %s: %s", out.Name(), err)
		}
	}(out)

	if newImg != img {
		// cropping or rendering changed the photo, save it as jpeg
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
		return nil, err
	}

	logrus.Debug("setting wallpaper...")
	err = setter.Set(wallpaperFileName)
	if err != nil {
		l.Error(err)
		return nil, err
	}

	return meta, err
}
