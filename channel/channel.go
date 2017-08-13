package channel

import (
	"bytes"
	"fmt"
	"image"
)

type Channel interface {
	Download() (*bytes.Reader, image.Image, string, error)
}

type channelMap map[string]Channel

func (m *channelMap) Register(name string, ch Channel) {
	(*m)[name] = ch
}

func (m channelMap) Run(name string) (*bytes.Reader, image.Image, string, error) {
	if ch, ok := (m)[name]; ok {
		return ch.Download()
	} else {
		return nil, nil, "", fmt.Errorf("channel %s not registered", name)
	}
}

var Channels = channelMap{}
