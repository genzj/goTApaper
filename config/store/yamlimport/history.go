package yamlimport

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config/store"
)

// legacyHistory mirrors the on-disk shape of history.json: a map from channel
// name to that channel's single most recent URL.
type legacyHistory struct {
	Meta    map[string]string `json:"Meta"`
	History map[string]struct {
		Name    string `json:"Name"`
		History string `json:"History"`
	} `json:"History"`
}

// importHistoryFile carries history.json into the wallpaper history and
// renames it.
//
// It exists only so an upgrade does not download the current picture again:
// deduplication reads the newest history entry of a channel, and without this
// every channel would start with none.
//
// The history is keyed by channel instance, but history.json keys are either
// an instance name or a channel *type* ("bing-wallpaper"), depending on the
// version that wrote them. See attribute for how one is resolved.
func importHistoryFile(s store.Store, path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var legacy legacyHistory
	if err := json.Unmarshal(data, &legacy); err != nil {
		// A corrupt history file costs at most one redundant download, which
		// is not worth failing the migration over.
		logrus.WithError(err).Warn("the history file is unreadable, starting with empty history")
		return archive(path)
	}

	// The file's age is the best available guess at when these pictures were
	// downloaded, and is closer than "now" for ordering them in the history.
	var downloadTime int64
	if info, err := os.Stat(path); err == nil {
		downloadTime = info.ModTime().Unix()
	}

	imported := 0
	err = s.Tx(func(tx store.Store) error {
		channels, err := tx.Channels()
		if err != nil {
			return err
		}

		for key, h := range legacy.History {
			if h.History == "" {
				continue
			}
			if h.Name != "" {
				key = h.Name
			}

			owner, ok := attribute(channels, key)
			if !ok {
				logrus.WithField("key", key).Debug(
					"cannot attribute legacy history to one channel, skipping it",
				)
				continue
			}

			// One entry per picture: two legacy keys can name the same URL.
			if _, err := tx.WallpaperHistoryEntryByURL(h.History); err == nil {
				continue
			}

			if _, err := tx.AppendWallpaperHistory(store.WallpaperHistoryEntry{
				ChannelName:  owner.Name,
				ChannelType:  owner.Type,
				URL:          h.History,
				DownloadTime: downloadTime,
			}); err != nil {
				return fmt.Errorf("history for %q: %w", key, err)
			}
			imported++
		}
		return nil
	})
	if err != nil {
		return err
	}

	logrus.WithField("entries", imported).Info("download history imported")
	return archive(path)
}

// attribute finds the channel instance a legacy history key belongs to.
//
// An instance with exactly that name wins. Otherwise the key is read as a
// channel type, which identifies an instance only when exactly one instance
// has that type: with several there is no telling which one the URL belonged
// to, and guessing wrong would suppress a download the user wanted. Such keys
// are skipped, costing at most one redundant download per channel.
//
// Mirrored by migration 0003, which carries the SQLite-era download_history
// over by the same rule.
func attribute(channels []store.ChannelInstance, key string) (store.ChannelInstance, bool) {
	var byType []store.ChannelInstance
	for _, c := range channels {
		if c.Name == key {
			return c, true
		}
		if c.Type == key {
			byType = append(byType, c)
		}
	}
	if len(byType) == 1 {
		return byType[0], true
	}
	return store.ChannelInstance{}, false
}
