package cmd

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/util"
)

// reapply downloads a recorded wallpaper from the URL the history kept and
// applies it again.
//
// It deliberately does not go through the channel that produced the picture:
// a channel resolves whatever its provider is serving *now*, which is the
// opposite of what re-applying an old entry means. The stored URL is fetched
// directly instead, and run through the same crop, watermark, write and set
// steps as a fresh download via applyPicture.
//
// Providers that serve time-limited URLs mean this can fail with a 403 or a
// 404 long after the entry was recorded. That is reported as an error and the
// current wallpaper is left alone; it is the accepted cost of keeping history
// re-applicable at all.
func reapply(entry store.WallpaperHistoryEntry) (*channel.PictureMeta, error) {
	l := logrus.WithFields(logrus.Fields{
		"channel": entry.ChannelName,
		"entry":   entry.ID,
	})

	if entry.URL == "" {
		return nil, fmt.Errorf("history entry %d has no source URL", entry.ID)
	}

	setterName := config.GetString(config.KeyWallpaperSetter)
	v, ok := setter.Setters.Get(setterName)
	if !ok {
		return nil, fmt.Errorf("setter %q is not registered", setterName)
	}
	wallpaperSetter := v.(setter.Setter)

	layers, err := currentStore().WatermarkLayers()
	if err != nil {
		l.WithError(err).Warn("cannot load watermark layers, continuing without them")
	}

	l.WithField("url", entry.URL).Debug("re-downloading a recorded wallpaper")
	resp, err := util.GetInType(entry.URL, "image/")
	if err != nil {
		return nil, fmt.Errorf(
			"cannot download %s again (the provider may have expired the link): %w",
			entry.URL, err,
		)
	}

	raw, img, format, err := util.DecodeFromResponse(resp)
	if err != nil {
		return nil, err
	}
	if raw == nil || img == nil {
		return nil, fmt.Errorf("no image at %s", entry.URL)
	}

	// Rebuilt from the stored row rather than reusing it, so the new history
	// entry records this application rather than duplicating the old one's
	// timestamp. The watermark templates read these fields, so they have to
	// carry the original's title and credit.
	meta := &channel.PictureMeta{
		Title:        entry.Title,
		Caption:      entry.Caption,
		Credit:       entry.Credit,
		Format:       format,
		Channel:      entry.ChannelType,
		ChannelKey:   entry.ChannelName,
		URL:          entry.URL,
		DownloadTime: time.Now(),
	}
	if entry.UploadTime != 0 {
		meta.UploadTime = time.Unix(entry.UploadTime, 0)
	}

	return applyPicture(l, raw, img, meta, wallpaperSetter, layers)
}
