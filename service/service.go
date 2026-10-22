// Package service holds the application operations a user interface needs,
// expressed as plain Go methods on JSON-tagged types.
//
// It is deliberately free of HTTP: the REST API in package api is a thin
// adapter over these methods, and a future desktop GUI can bind the same
// methods directly rather than going through a socket.
package service

import (
	"fmt"
	"slices"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// Redacted stands in for a secret value in anything the service hands out.
//
// A client may write it back unchanged to mean "leave this value alone", which
// is what lets a form round-trip a channel without ever being told its API
// key.
const Redacted = "********"

// Config exposes configuration reads and writes.
type Config struct {
	store store.Store
}

// NewConfig builds a configuration service over a store.
func NewConfig(s store.Store) *Config {
	return &Config{store: s}
}

// SettingView is one setting together with the schema describing it, so a
// client can render the right control without hard-coding the vocabulary.
type SettingView struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	IsDefault   bool     `json:"isDefault"`
	Kind        string   `json:"kind"`
	Default     string   `json:"default"`
	EnumValues  []string `json:"enumValues,omitempty"`
	Description string   `json:"description,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
}

// Settings returns every setting, with secrets redacted.
func (c *Config) Settings() ([]SettingView, error) {
	settings, err := c.store.Settings()
	if err != nil {
		return nil, err
	}

	out := make([]SettingView, 0, len(settings))
	for _, s := range settings {
		def, _ := config.Lookup(s.Key)
		value := s.Value
		if def.Secret && value != "" {
			value = Redacted
		}
		out = append(out, SettingView{
			Key:         s.Key,
			Value:       value,
			IsDefault:   s.IsDefault,
			Kind:        string(def.Kind),
			Default:     def.Default,
			EnumValues:  def.EnumValues,
			Description: def.Description,
			Secret:      def.Secret,
		})
	}
	return out, nil
}

// SettingDefinitions returns the settings schema on its own, so a client can
// build a form before it has any values.
func (c *Config) SettingDefinitions() []config.Definition {
	return config.Definitions()
}

// SetSetting validates and stores one setting. Writing back the redaction
// sentinel is a no-op, so a client can submit a whole form unchanged.
func (c *Config) SetSetting(key, value string) error {
	if def, ok := config.Lookup(key); ok && def.Secret && value == Redacted {
		return nil
	}
	return c.store.SetSetting(key, value)
}

// UnsetSetting reverts one setting to its default.
func (c *Config) UnsetSetting(key string) error {
	return c.store.UnsetSetting(key)
}

// ChannelView is a channel instance as presented to a client: its own
// configuration plus the options it actually resolves to.
type ChannelView struct {
	store.ChannelInstance
	// EffectiveOptions is the channel's options with its template applied.
	// Read-only; writes go to the instance's own fields.
	EffectiveOptions map[string]any `json:"effectiveOptions"`
}

// Channels returns every configured channel in refresh order.
func (c *Config) Channels() ([]ChannelView, error) {
	channels, err := c.store.Channels()
	if err != nil {
		return nil, err
	}

	out := make([]ChannelView, 0, len(channels))
	for _, ch := range channels {
		eff, err := c.store.EffectiveChannel(ch.Name)
		if err != nil {
			return nil, err
		}
		ch.Options = redactOptions(ch.Options)
		out = append(out, ChannelView{
			ChannelInstance:  ch,
			EffectiveOptions: redactOptions(eff.Options),
		})
	}
	return out, nil
}

// Channel returns one channel by name.
func (c *Config) Channel(name string) (ChannelView, error) {
	ch, err := c.store.Channel(name)
	if err != nil {
		return ChannelView{}, err
	}
	eff, err := c.store.EffectiveChannel(name)
	if err != nil {
		return ChannelView{}, err
	}
	ch.Options = redactOptions(ch.Options)
	return ChannelView{
		ChannelInstance:  ch,
		EffectiveOptions: redactOptions(eff.Options),
	}, nil
}

// UpsertChannel creates or updates a channel.
//
// Options holding the redaction sentinel keep their stored value, so a client
// that round-trips a channel it was shown cannot blank the API key it was
// never given.
func (c *Config) UpsertChannel(ch store.ChannelInstance) error {
	if err := c.validateChannel(ch); err != nil {
		return err
	}

	existing, err := c.store.Channel(ch.Name)
	if err == nil {
		ch.Options = restoreRedacted(ch.Options, existing.Options)
		if ch.ID == 0 {
			ch.ID = existing.ID
		}
	} else {
		// A new channel goes last rather than displacing the first one.
		if ch.SortOrder == 0 {
			ch.SortOrder = store.AppendOrder
		}
	}

	return c.store.UpsertChannel(ch)
}

// validateChannel checks a channel against its type's option schema, so a
// misconfiguration is reported at save time rather than at download time.
func (c *Config) validateChannel(ch store.ChannelInstance) error {
	schema, ok := channel.Channels.Schema(ch.Type)
	if !ok {
		if _, registered := channel.Channels.Get(ch.Type); !registered {
			return fmt.Errorf(
				"%w: channel type %q", store.ErrNotFound, ch.Type,
			)
		}
		// Registered without a schema: nothing to validate against.
		return nil
	}

	// The effective options are what matter, since a template may supply a
	// value the instance omits.
	effective := map[string]any{}
	if ch.TemplateName != "" {
		t, err := c.store.ChannelTemplate(ch.TemplateName)
		if err != nil {
			return err
		}
		for k, v := range t.Options {
			effective[k] = v
		}
	}
	for k, v := range ch.Options {
		effective[k] = v
	}
	if ch.Strategy != nil {
		effective["strategy"] = *ch.Strategy
	}

	for _, o := range schema.Options {
		value, present := effective[o.Key]
		if !present {
			if o.Required {
				return fmt.Errorf("channel %q is missing the required option %q", ch.Name, o.Key)
			}
			continue
		}
		if o.Kind == channel.OptionEnum && len(o.EnumValues) > 0 {
			s, _ := value.(string)
			if !slices.Contains(o.EnumValues, s) {
				return &config.InvalidValueError{
					Key:      ch.Name + "." + o.Key,
					Value:    fmt.Sprintf("%v", value),
					Kind:     config.KindEnum,
					Expected: o.EnumValues,
				}
			}
		}
	}
	return nil
}

// DeleteChannel removes a channel and its history.
func (c *Config) DeleteChannel(name string) error {
	return c.store.DeleteChannel(name)
}

// ReorderChannels sets the order channels are attempted in.
func (c *Config) ReorderChannels(names []string) error {
	return c.store.ReorderChannels(names)
}

// ChannelTypes lists the available channel types with their option schemas.
func (c *Config) ChannelTypes() []channel.TypeInfo {
	return channel.Channels.Types()
}

// TemplateView is a channel template plus the channels inheriting from it, so
// a client can warn before a delete that is going to be refused.
type TemplateView struct {
	store.ChannelTemplate
	UsedBy []string `json:"usedBy"`
}

// ChannelTemplates returns every template.
func (c *Config) ChannelTemplates() ([]TemplateView, error) {
	templates, err := c.store.ChannelTemplates()
	if err != nil {
		return nil, err
	}

	out := make([]TemplateView, 0, len(templates))
	for _, t := range templates {
		users, err := c.store.ChannelsUsingTemplate(t.Name)
		if err != nil {
			return nil, err
		}
		t.Options = redactOptions(t.Options)
		out = append(out, TemplateView{ChannelTemplate: t, UsedBy: users})
	}
	return out, nil
}

// ChannelTemplate returns one template by name.
func (c *Config) ChannelTemplate(name string) (TemplateView, error) {
	t, err := c.store.ChannelTemplate(name)
	if err != nil {
		return TemplateView{}, err
	}
	users, err := c.store.ChannelsUsingTemplate(name)
	if err != nil {
		return TemplateView{}, err
	}
	t.Options = redactOptions(t.Options)
	return TemplateView{ChannelTemplate: t, UsedBy: users}, nil
}

// UpsertChannelTemplate creates or updates a template, preserving any secret
// the client submitted as the redaction sentinel.
func (c *Config) UpsertChannelTemplate(t store.ChannelTemplate) error {
	if existing, err := c.store.ChannelTemplate(t.Name); err == nil {
		t.Options = restoreRedacted(t.Options, existing.Options)
	}
	return c.store.UpsertChannelTemplate(t)
}

// DeleteChannelTemplate deletes a template that nothing inherits from. It
// returns store.ErrTemplateInUse if channels still reference it.
func (c *Config) DeleteChannelTemplate(name string) error {
	return c.store.DeleteChannelTemplate(name)
}

// InlineChannelTemplate copies a template's options into every channel that
// inherits it and then deletes it, leaving their effective configuration
// unchanged. This is the supported way to remove a template that is in use.
func (c *Config) InlineChannelTemplate(name string) error {
	return c.store.InlineChannelTemplate(name)
}

// WatermarkLayers returns the configured watermark overlays in render order.
func (c *Config) WatermarkLayers() ([]store.WatermarkLayer, error) {
	return c.store.WatermarkLayers()
}

// UpsertWatermarkLayer creates or updates one watermark layer.
func (c *Config) UpsertWatermarkLayer(l store.WatermarkLayer) error {
	if l.ID == 0 && l.SortOrder == 0 {
		l.SortOrder = store.AppendOrder
	}
	return c.store.UpsertWatermarkLayer(l)
}

// DeleteWatermarkLayer removes one watermark layer.
func (c *Config) DeleteWatermarkLayer(id int64) error {
	return c.store.DeleteWatermarkLayer(id)
}

// redactOptions copies an option map with every secret value masked.
func redactOptions(options map[string]any) map[string]any {
	if options == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(options))
	for k, v := range options {
		if isSecretOption(k) && v != nil && v != "" {
			out[k] = Redacted
			continue
		}
		out[k] = v
	}
	return out
}

// restoreRedacted replaces sentinel values in submitted with whatever is
// already stored, so a client can save a form without having seen the secrets
// in it.
func restoreRedacted(submitted, stored map[string]any) map[string]any {
	if submitted == nil {
		return nil
	}
	out := make(map[string]any, len(submitted))
	for k, v := range submitted {
		if s, ok := v.(string); ok && s == Redacted {
			if prev, had := stored[k]; had {
				out[k] = prev
				continue
			}
			// Nothing stored to restore: drop it rather than persisting the
			// mask as if it were a real key.
			continue
		}
		out[k] = v
	}
	return out
}

// isSecretOption reports whether any registered channel type marks this option
// key as secret.
func isSecretOption(key string) bool {
	for _, t := range channel.Channels.Types() {
		if o, ok := t.Schema.Lookup(key); ok && o.Secret {
			return true
		}
	}
	return false
}
