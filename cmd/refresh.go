package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/actor"
	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/actor/watermark"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/history"
)

var errNoAvailableChannel error = fmt.Errorf("no available channel")

var refreshCmd = &cobra.Command{
	Use:   "refresh [channel...]",
	Short: "Trigger pic downloading and wallpaper setting",
	Long: `Download a wallpaper and set it.

With a daemon running, the daemon does the download and this returns as soon
as it has started; it is refused while another download is in progress. Two
processes downloading at once would race to write the same wallpaper file.
Use --local to download in this process regardless.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !refreshLocal {
			if client := daemonClient(); client != nil {
				if err := client.Refresh(args, force, nil); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				fmt.Println("refreshing in the background; follow it with \"goTApaper logs -f\"")
				return
			}
		}
		if _, err := refresh(args, force); err != nil {
			os.Exit(1)
		}
	},
}

var (
	force        bool
	refreshLocal bool
)

func init() {
	refreshCmd.PersistentFlags().String(
		"setter", "", "setter to configure desktop wallpaper",
	)
	refreshCmd.PersistentFlags().BoolVar(
		&force, "force", false, "ignore history and always download",
	)
	refreshCmd.PersistentFlags().BoolVar(
		&refreshLocal, "local", false,
		"download in this process even if a daemon is running",
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

// refresh downloads and applies a wallpaper. force downloads even a picture
// the channel applied last time. It is a parameter rather than the --force
// flag read directly, because the daemon runs it for API requests that each
// bring their own.
func refresh(specifiedChannels []string, force bool) (*channel.PictureMeta, error) {
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

		// Both are decided per run, so they are layered on top of the stored
		// options rather than written to them. The last applied URL is what
		// lets a channel skip a picture it already set.
		lastURL, err := currentStore().LastAppliedURL(ch.Name)
		if err != nil {
			l.WithError(err).Warn("cannot read the last applied picture, downloading regardless")
		}
		setting := ch.Settings(map[string]any{
			channel.OptionKeyForce:          force,
			channel.OptionKeyLastAppliedURL: lastURL,
		})
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

	return applyPicture(l, raw, img, meta, wallpaperSetter, layers, nil)
}

// applyPicture crops, watermarks, writes and sets a downloaded picture, then
// records it in the history. reapplied is the history entry the picture came
// from when it is being applied again, and nil for a fresh download.
//
// Split out of detectOneChannel so that re-applying a history entry runs the
// same steps rather than a second implementation that could drift: a re-applied
// wallpaper must be cropped and watermarked exactly as the original was.
func applyPicture(
	l *logrus.Entry,
	raw *bytes.Reader,
	img image.Image,
	meta *channel.PictureMeta,
	wallpaperSetter setter.Setter,
	layers []store.WatermarkLayer,
	reapplied *store.WallpaperHistoryEntry,
) (*channel.PictureMeta, error) {
	newImg := actor.DefaultCropper.Crop(img)

	newImg, _ = watermark.Render(newImg, meta, layers)

	wallpaperFileName := config.GetWallpaperFileName() + "." + meta.Format

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

	// Recorded only after the wallpaper is actually on the desktop, so the
	// history is a log of what was applied rather than of what was downloaded.
	recordHistory(l, meta, img, wallpaperFileName, reapplied)

	return meta, err
}

// recordHistory records a wallpaper that has just been applied, writes its
// thumbnail, and enforces the entry cap.
//
// A picture already in the history -- re-applied, or downloaded again -- updates
// its entry instead of appending a second one: the history lists pictures,
// and one picture applied twice is one entry applied twice, not two entries.
//
// Every failure here is logged and swallowed. The wallpaper is already set,
// which is the user's actual goal; losing a history row is a far smaller
// problem than turning a successful refresh into a failed one.
func recordHistory(
	l *logrus.Entry, meta *channel.PictureMeta, img image.Image, wallpaperFileName string,
	reapplied *store.WallpaperHistoryEntry,
) {
	now := time.Now().Unix()

	// A fresh download of a picture already in the history -- a forced
	// refresh, or a provider serving the same photo again -- is that picture
	// applied again, not a new one. Pictures are matched by URL only: nothing
	// compares the images, so one picture under two URLs is still two entries.
	if reapplied == nil && meta.URL != "" {
		if existing, err := currentStore().WallpaperHistoryEntryByURL(meta.URL); err == nil {
			l.WithField("entry", existing.ID).Info(
				"this picture is already in the history, updating its entry",
			)
			reapplied = &existing
		}
	}

	if reapplied != nil {
		if markReapplied(l, img, wallpaperFileName, reapplied, now) {
			sweepHistory(l)
			return
		}
		// The entry went while it was being re-applied -- deleted, or pruned.
		// The picture is on the desktop regardless, so record it afresh.
	}

	entry := store.WallpaperHistoryEntry{
		ChannelName:  meta.ChannelKey,
		ChannelType:  meta.Channel,
		URL:          meta.URL,
		Title:        meta.Title,
		Caption:      meta.Caption,
		Credit:       meta.Credit,
		Format:       meta.Format,
		DownloadTime: now,
		FilePath:     wallpaperFileName,
	}
	if !meta.UploadTime.IsZero() {
		entry.UploadTime = meta.UploadTime.Unix()
	}

	// The thumbnail is written before the row so the row can name it, and is
	// named after the download time rather than the row id, which does not
	// exist yet. A file whose row never materialises is reclaimed by the
	// orphan sweep below.
	entry.ThumbPath = writeHistoryThumbnail(l, img, meta.ChannelKey, now)

	if _, err := currentStore().AppendWallpaperHistory(entry); err != nil {
		l.WithError(err).Warn("cannot record the wallpaper history")
		return
	}

	sweepHistory(l)
}

// markReapplied updates the entry a picture was re-applied from, reporting
// whether it still existed.
//
// Its thumbnail is kept: it is the same picture. One is written only if the
// entry has none on disk -- recorded before thumbnails were on, or reclaimed
// since -- so re-applying also repairs a history entry with nothing to show.
func markReapplied(
	l *logrus.Entry, img image.Image, wallpaperFileName string,
	entry *store.WallpaperHistoryEntry, now int64,
) bool {
	thumb := ""
	if entry.ThumbPath == "" || !fileExists(entry.ThumbPath) {
		thumb = writeHistoryThumbnail(l, img, entry.ChannelName, now)
	}

	err := currentStore().MarkReapplied(entry.ID, now, wallpaperFileName, thumb)
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		l.WithError(err).Warn("cannot record the re-applied wallpaper in the history")
	}
	return true
}

// writeHistoryThumbnail writes a thumbnail for the history, returning its
// path, or "" when thumbnails are off or it could not be written.
func writeHistoryThumbnail(l *logrus.Entry, img image.Image, channelName string, at int64) string {
	if !config.GetBool(config.KeyHistoryThumbnails) {
		return ""
	}
	name := fmt.Sprintf("%s-%d.jpg", channelName, at)
	path := filepath.Join(config.GetThumbnailDir(), name)
	written, err := actor.WriteThumbnail(img, path, config.DefaultThumbnailWidth)
	if err != nil {
		l.WithError(err).Warn("cannot write the history thumbnail")
		return ""
	}
	return written
}

// sweepHistory enforces the history entry cap and reclaims unreferenced
// thumbnails. Cheap enough -- one indexed delete and one directory listing --
// to run after every refresh rather than on a timer of its own.
func sweepHistory(l *logrus.Entry) {
	result, err := history.Sweep(currentStore(), history.SweepOptions{
		KeepAtMost:   config.GetInt(config.KeyHistoryMaxEntries),
		ThumbnailDir: config.GetThumbnailDir(),
		// Turning thumbnails off is what removes the ones already written;
		// there is no separate purge command to remember.
		PurgeThumbnails: !config.GetBool(config.KeyHistoryThumbnails),
	})
	if err != nil {
		l.WithError(err).Warn("cannot prune the wallpaper history")
		return
	}
	if result.RowsRemoved > 0 || result.FilesRemoved > 0 {
		l.WithFields(logrus.Fields{
			"rows":  result.RowsRemoved,
			"files": result.FilesRemoved,
			"bytes": result.BytesReclaimed,
		}).Debug("pruned the wallpaper history")
	}
}
