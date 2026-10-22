package channel

import (
	"bytes"
	"fmt"
	"image"
	"sort"
	"time"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/util"
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

// Settings provides read access to one channel instance's effective
// configuration: its own options with any inherited template already applied.
//
// This is deliberately a narrow interface rather than a concrete type. The
// method set is exactly what the bundled channels read, so the store can
// satisfy it over a database row while tests pass a plain map.
type Settings interface {
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	GetStringMapString(key string) map[string]string
	IsSet(key string) bool
}

// Channel defines a wallpaper downloader
type Channel interface {
	Download(Settings) (*bytes.Reader, image.Image, *PictureMeta, error)
}

// Option describes one configurable setting of a channel type.
//
// It is config.ValueSpec, the same type global settings use: both describe "a
// configurable value with a kind, a default and maybe an enum", and keeping
// two near-identical definitions meant two places to extend and two
// validators free to disagree about what a valid enum is.
type Option = config.ValueSpec

// OptionKind classifies an option's type, so a UI can pick a control and the
// store can validate before a download is attempted.
type OptionKind = config.Kind

// Option kinds, aliased so a channel declaring its schema does not have to
// reach into the config package for them.
const (
	OptionString    = config.KindString
	OptionInt       = config.KindInt
	OptionBool      = config.KindBool
	OptionEnum      = config.KindEnum
	OptionStringMap = config.KindStringMap
)

// OptionSchema is a channel type's full set of options.
//
// Each channel declares its own, which is what lets a UI render a form for a
// channel it knows nothing about. It also expresses variation no shared table
// could: the strategies bing accepts are not the ones unsplash accepts.
type OptionSchema struct {
	// Version is the option shape's version, recorded against every instance
	// that is written. Bump it when an option is renamed or removed, or when
	// the meaning of a stored value changes.
	//
	// It exists because upgrading a downloader can change its options in ways
	// a SQL migration cannot express -- splitting one value into two, say, or
	// reinterpreting a string. Storing the version each instance was written
	// against lets the channel recognise an old row and convert it in Go.
	// Zero is treated as 1, so a channel that has never changed shape need not
	// set it.
	Version int `json:"version,omitempty"`

	Options []Option `json:"options"`
}

// SchemaVersion returns the schema's version, treating zero as the first.
func (s OptionSchema) SchemaVersion() int {
	if s.Version == 0 {
		return 1
	}
	return s.Version
}

// Lookup finds an option by key.
func (s OptionSchema) Lookup(key string) (Option, bool) {
	for _, o := range s.Options {
		if o.Key == key {
			return o, true
		}
	}
	return Option{}, false
}

type channelMap struct {
	util.RegistryMap
	schemas map[string]OptionSchema
}

func (m *channelMap) Register(name string, ch Channel) {
	m.RegistryMap.Register(name, ch)
}

// RegisterWithSchema registers a channel along with the options it accepts.
// Channels should prefer this over Register so their settings are
// introspectable.
func (m *channelMap) RegisterWithSchema(name string, ch Channel, schema OptionSchema) {
	m.RegistryMap.Register(name, ch)
	if m.schemas == nil {
		m.schemas = map[string]OptionSchema{}
	}
	m.schemas[name] = schema
}

// Schema returns the option schema of a channel type.
func (m channelMap) Schema(name string) (OptionSchema, bool) {
	s, ok := m.schemas[name]
	return s, ok
}

// Types lists every registered channel type with its option schema, sorted by
// name. This is what a configuration UI reads to build its forms.
func (m channelMap) Types() []TypeInfo {
	names := m.RegistryMap.Names()
	out := make([]TypeInfo, 0, len(names))
	for _, name := range names {
		out = append(out, TypeInfo{Name: name, Schema: m.schemas[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// TypeInfo pairs a channel type's registry name with its option schema.
type TypeInfo struct {
	Name   string       `json:"name"`
	Schema OptionSchema `json:"schema"`
}

func (m channelMap) Run(name string, setting Settings) (*bytes.Reader, image.Image, *PictureMeta, error) {
	if v, ok := m.Get(name); ok {
		ch := v.(Channel)
		return ch.Download(setting)
	}
	return nil, nil, nil, fmt.Errorf("channel %s not registered", name)
}

// Channels handles all registered channels
// TODO make this variable local and add functions instead
var Channels = channelMap{RegistryMap: util.RegistryMap{}, schemas: map[string]OptionSchema{}}

// Common option keys shared by most channels.
const (
	// OptionKeyForce is injected per refresh rather than stored, so it is
	// absent from every channel's schema.
	OptionKeyForce = "force"

	OptionKeyStrategy = "strategy"
	OptionKeyWidth    = "width"
	OptionKeyHeight   = "height"
	OptionKeyAPIKey   = "key"
)

// apiKeyOption describes the API key option shared by the keyed providers.
func apiKeyOption(signupURL string) Option {
	return Option{
		Key:         OptionKeyAPIKey,
		Label:       "API key",
		Kind:        OptionString,
		Required:    true,
		Secret:      true,
		Description: "API access key; create one at " + signupURL,
	}
}
