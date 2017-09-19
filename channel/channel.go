package channel

import (
	"bytes"
	"fmt"
	"github.com/genzj/goTApaper/util"
	"image"
)

type Channel interface {
	Download() (*bytes.Reader, image.Image, string, error)
}

type channelMap struct {
	util.RegistryMap
}

func (m *channelMap) Register(name string, ch Channel) {
	m.RegistryMap.Register(name, ch)
}

func (m channelMap) Run(name string) (*bytes.Reader, image.Image, string, error) {
	if v, ok := m.Get(name); ok {
		ch := v.(Channel)
		return ch.Download()
	} else {
		return nil, nil, "", fmt.Errorf("channel %s not registered", name)
	}
}

var Channels = channelMap{RegistryMap: util.RegistryMap{}}
