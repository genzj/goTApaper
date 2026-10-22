package store

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/history"
)

// HistoryManager keeps download records in the configuration database.
//
// It implements history.Manager, the same interface the JSON file manager it
// replaces did, with the same semantics: one URL per channel, overwritten on
// each successful download. Channel call sites (Load, Has, Mark) are unchanged.
//
// The adapter lives here rather than in the history package so that the
// dependency runs store -> history. The other direction would be a cycle, as
// channels depend on history and the store's tests depend on channels.
type HistoryManager struct {
	store Store
}

var _ history.Manager = (*HistoryManager)(nil)

// NewHistoryManager wraps a store.
func NewHistoryManager(s Store) *HistoryManager {
	return &HistoryManager{store: s}
}

// Load reads a channel's history, returning an empty record when the channel
// has never downloaded anything.
func (m *HistoryManager) Load(name string) (*history.History, error) {
	e, err := m.store.History(name)
	if errors.Is(err, ErrNotFound) {
		return history.NewHistory(name), nil
	}
	if err != nil {
		logrus.WithField("channel", name).WithError(err).Error("cannot load history")
		return nil, err
	}
	return &history.History{Name: name, History: e.URL}, nil
}

// Save writes a channel's history back.
func (m *HistoryManager) Save(h *history.History) error {
	if h == nil || h.Name == "" {
		return nil
	}
	return m.store.PutHistory(HistoryEntry{ChannelName: h.Name, URL: h.History})
}
