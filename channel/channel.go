package channel

import (
	"bytes"
	"fmt"
	"image"

	"github.com/genzj/goTApaper/util"
)

type Channel interface {
	Download(force bool) (*bytes.Reader, image.Image, string, error)
}

type channelMap struct {
	util.RegistryMap
}

func (m *channelMap) Register(name string, ch Channel) {
	m.RegistryMap.Register(name, ch)
}

func (m channelMap) Run(name string, force bool) (*bytes.Reader, image.Image, string, error) {
	if v, ok := m.Get(name); ok {
		ch := v.(Channel)
		return ch.Download(force)
	} else {
		return nil, nil, "", fmt.Errorf("channel %s not registered", name)
	}
}

var Channels = channelMap{RegistryMap: util.RegistryMap{}}
