package store

import (
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config"
)

// Provider reads scalar settings from a Store, with a layer of in-memory
// overrides consulted first.
//
// Overrides are how command-line flags work. A flag has to beat the stored
// value for the run it was passed on, but it must never be written to the
// database: `goTApaper --debug refresh` should not leave debug logging switched
// on for the daemon afterwards.
type Provider struct {
	store Store

	mu        sync.RWMutex
	overrides map[string]string
}

var _ config.Provider = (*Provider)(nil)

// NewProvider wraps a store.
func NewProvider(s Store) *Provider {
	return &Provider{store: s, overrides: map[string]string{}}
}

// Override sets a transient value for key, validated against the schema but
// never persisted. An empty key is ignored so callers can pass through unset
// flags without a guard.
func (p *Provider) Override(key, value string) error {
	if key == "" {
		return nil
	}
	if err := config.Validate(key, value); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.overrides[key] = value
	return nil
}

// ClearOverride removes a transient value.
func (p *Provider) ClearOverride(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.overrides, key)
}

// Store exposes the underlying store.
func (p *Provider) Store() Store { return p.store }

// raw resolves a key: override first, then the stored value, then the schema
// default.
func (p *Provider) raw(key string) string {
	p.mu.RLock()
	v, ok := p.overrides[key]
	p.mu.RUnlock()
	if ok {
		return v
	}

	s, err := p.store.Setting(key)
	if err != nil {
		// A missing key is a programming error, not a user error, so it is
		// logged rather than surfaced through these non-erroring getters.
		logrus.WithField("key", key).WithError(err).Debug("cannot read setting")
		if d, ok := config.Lookup(key); ok {
			return d.Default
		}
		return ""
	}
	return s.Value
}

// GetString reads a string setting.
func (p *Provider) GetString(key string) string { return p.raw(key) }

// GetBool reads a boolean setting.
func (p *Provider) GetBool(key string) bool {
	b, err := strconv.ParseBool(p.raw(key))
	return err == nil && b
}

// GetInt reads an integer setting.
func (p *Provider) GetInt(key string) int {
	i, err := strconv.Atoi(p.raw(key))
	if err != nil {
		return 0
	}
	return i
}

// GetFloat64 reads a float setting.
func (p *Provider) GetFloat64(key string) float64 {
	f, err := strconv.ParseFloat(p.raw(key), 64)
	if err != nil {
		return 0
	}
	return f
}
