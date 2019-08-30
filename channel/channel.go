package channel

import (
	"bytes"
	"fmt"
	"image"

	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
)

// Channel defines a wallpaper downloader
type Channel interface {
	Download(*viper.Viper) (*bytes.Reader, image.Image, string, error)
}

type channelMap struct {
	util.RegistryMap
}

func (m *channelMap) Register(name string, ch Channel) {
	m.RegistryMap.Register(name, ch)
}

func (m channelMap) Run(name string, setting *viper.Viper) (*bytes.Reader, image.Image, string, error) {
	if v, ok := m.Get(name); ok {
		ch := v.(Channel)
		return ch.Download(setting)
	}
	return nil, nil, "", fmt.Errorf("channel %s not registered", name)
}

// Channels handles all registered channels
// TODO make this variable local and add functions instead
var Channels = channelMap{RegistryMap: util.RegistryMap{}}
