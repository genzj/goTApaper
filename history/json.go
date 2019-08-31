package history

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/genzj/goTApaper/config"

	"github.com/sirupsen/logrus"
)

type skeleton struct {
	Meta    map[string]string
	History map[string]History
}

// JSONHistoryManager keeps downloading records in JSON file
type JSONHistoryManager struct {
	skeleton skeleton
}

// Load a JSON history file from disk
func (m JSONHistoryManager) Load(name string) (*History, error) {
	fn := config.GetHistoryFileName()
	file, e := ioutil.ReadFile(fn)

	if e != nil && os.IsNotExist(e) {
		return NewHistory(name), nil
	} else if e != nil {
		logrus.Errorf("error on loading history file: %s", e)
		return nil, e
	}

	if e := json.Unmarshal(file, &m.skeleton); e != nil {
		logrus.WithField("error", e).Warnln("corrupted history file")
		// ignore error, maybe corrupted file, expect next save
		// may correct it.
		return NewHistory(name), nil
	}

	if h, ok := m.skeleton.History[name]; ok {
		return &h, nil
	} else {
		return NewHistory(name), nil
	}
}

// Save to disk file
func (m *JSONHistoryManager) Save(h *History) error {
	fn := config.GetHistoryFileName()

	m.skeleton.History[h.Name] = *h
	if bs, err := json.Marshal(m.skeleton); err != nil {
		logrus.WithField("error", err).Errorln("cannot save history file")
		return err
	} else {
		return ioutil.WriteFile(fn, bs, os.FileMode(0644))
	}
}

// JSONHistoryManagerSingleton is the default instance
var JSONHistoryManagerSingleton = &JSONHistoryManager{
	skeleton: skeleton{
		Meta:    make(map[string]string),
		History: make(map[string]History),
	},
}
