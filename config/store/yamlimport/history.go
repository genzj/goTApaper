package yamlimport

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config/store"
)

// legacyHistory mirrors the on-disk shape of history.json: a map from channel
// name to that channel's single most recent URL. The database keeps the same
// one-row-per-channel structure, so this is a direct translation.
type legacyHistory struct {
	Meta    map[string]string `json:"Meta"`
	History map[string]struct {
		Name    string `json:"Name"`
		History string `json:"History"`
	} `json:"History"`
}

// importHistoryFile copies history.json into download_history and renames it.
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

	err = s.Tx(func(tx store.Store) error {
		for name, h := range legacy.History {
			if h.History == "" {
				continue
			}
			channelName := name
			if h.Name != "" {
				channelName = h.Name
			}
			if err := tx.PutHistory(store.HistoryEntry{
				ChannelName: channelName,
				URL:         h.History,
			}); err != nil {
				return fmt.Errorf("history for %q: %w", channelName, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	logrus.WithField("entries", len(legacy.History)).Info("download history imported")
	return archive(path)
}
