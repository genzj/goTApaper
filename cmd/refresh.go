package cmd

import (
	"fmt"
	"image/jpeg"
	"math/rand"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/actor/watermark"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

var errNoAvailableChannel error = fmt.Errorf("no available channel")

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
	refreshCmd.PersistentFlags().String(
		"setter", "", "setter to configure desktop wallpaper",
	)
	refreshCmd.PersistentFlags().BoolVar(
		&force, "force", false, "ignore history and always download",
	)
	RootCmd.AddCommand(refreshCmd)
}

// collectActiveChannels returns the enabled channels in refresh order.
//
// Enablement, probability and order are columns, so this is a single ordered
// query rather than a walk over a loosely typed list.
func collectActiveChannels() ([]store.EffectiveChannel, error) {
	channels, err := currentStore().EffectiveChannels()
	if err != nil {
		return nil, err
	}

	if len(channels) == 0 {
		logrus.Warn("no enabled channels found; run \"goTApaper config channel list\"")
		return nil, nil
	}

	// The last channel always runs, so that every refresh makes at least one
	// attempt however the probabilities are set.
	channels[len(channels)-1].Probability = 1.0

	logrus.Debugf("active channels: %+v", channels)
	return channels, nil
}

// collectSpecifiedChannels resolves channels named on the command line. They
// run unconditionally, and may be disabled ones.
func collectSpecifiedChannels(names []string) ([]store.EffectiveChannel, error) {
	if len(names) == 0 {
		return nil, nil
	}

	s := currentStore()
	out := make([]store.EffectiveChannel, 0, len(names))
	for _, name := range names {
		c, err := s.EffectiveChannel(name)
		if err != nil {
			return nil, err
		}
		c.Probability = 1.0
		out = append(out, c)
	}
	logrus.Debugf("specified channels in args: %+v", out)
	return out, nil
}

func refresh(specifiedChannels []string) (*channel.PictureMeta, error) {
	// No config reload here: the store is read live, and a change made through
	// the API or the CLI emits an event the daemon already listens for.
	activeChannels, err := collectSpecifiedChannels(specifiedChannels)
	if err != nil {
		logrus.WithError(err).Error("cannot resolve the requested channels")
		return nil, err
	}

	if len(activeChannels) == 0 {
		if activeChannels, err = collectActiveChannels(); err != nil {
			logrus.WithError(err).Error("cannot load channel configuration")
			return nil, err
		}
	}
	if len(activeChannels) == 0 {
		return nil, errNoAvailableChannel
	}

	setterName := config.GetString(config.KeyWallpaperSetter)
	v, ok := setter.Setters.Get(setterName)
	if !ok {
		logrus.Panicf("setter \"%s\" not registered", setterName)
	}
	wallpaperSetter := v.(setter.Setter)

	layers, err := currentStore().WatermarkLayers()
	if err != nil {
		logrus.WithError(err).Warn("cannot load watermark layers, continuing without them")
	}

	for _, ch := range activeChannels {
		l := logrus.WithField("channel", ch.Name)

		dice := rand.Float32()
		if ch.Probability < 1 && dice > ch.Probability {
			l.WithField("dice", dice).WithField(
				"probability", ch.Probability,
			).Info("skipped randomly")
			continue
		}

		// "force" is decided per run, so it is layered on top of the stored
		// options rather than written to them.
		setting := ch.Settings(map[string]any{channel.OptionKeyForce: force})
		l.Debugf("setting: %+v", ch.Options)

		if meta, err := detectOneChannel(ch, setting, wallpaperSetter, layers); err != nil || meta == nil {
			continue
		} else {
			// Exit on the first success. The remaining channels get their turn
			// on the next schedule, and the history mechanism keeps them from
			// repeating a picture.
			return meta, err
		}
	}
	return nil, errNoAvailableChannel
}

func detectOneChannel(
	ch store.EffectiveChannel,
	setting channel.Settings,
	wallpaperSetter setter.Setter,
	layers []store.WatermarkLayer,
) (*channel.PictureMeta, error) {
	l := logrus.WithField("channel", ch.Name)
	wallpaperPath := config.GetWallpaperFileName()

	raw, img, meta, err := channel.Channels.Run(ch.Type, setting)
	if err != nil {
		l.Error(err)
		return nil, err
	}

	if meta != nil {
		meta.Channel = ch.Type
		meta.ChannelKey = ch.Name
		l.Debugf("picture metadata %+v", meta)
	} else {
		l.Warn("no picture metadata")
		return nil, err
	}

	if raw == nil || img == nil {
		l.Infoln("no image downloaded")
		return nil, err
	}

	newImg := actor.DefaultCropper.Crop(img)

	newImg, _ = watermark.Render(newImg, meta, layers)

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
	err = wallpaperSetter.Set(wallpaperFileName)
	if err != nil {
		l.Error(err)
		return nil, err
	}

	return meta, err
}
