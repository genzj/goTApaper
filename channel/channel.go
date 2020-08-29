package channel

import (
	"bytes"
	"fmt"
	"image"
	"time"

	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
)

// PictureMeta contains title, format, time and other metadata of a picture
type PictureMeta struct {
	Title        string
	Caption      string
	Credit       string
	Format       string
	Channel      string
	ChannelKey   string
	UploadTime   time.Time
	DownloadTime time.Time
}

// Channel defines a wallpaper downloader
type Channel interface {
	Download(*viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error)
}

type channelMap struct {
	util.RegistryMap
}

func (m *channelMap) Register(name string, ch Channel) {
	m.RegistryMap.Register(name, ch)
}

func (m channelMap) Run(name string, setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
	if v, ok := m.Get(name); ok {
		ch := v.(Channel)
		return ch.Download(setting)
	}
	return nil, nil, nil, fmt.Errorf("channel %s not registered", name)
}

// Channels handles all registered channels
// TODO make this variable local and add functions instead
var Channels = channelMap{RegistryMap: util.RegistryMap{}}
