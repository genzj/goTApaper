package config

import (
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"
)

// Provider reads scalar configuration values. It is the narrow view of the
// store that the download pipeline needs -- the proxy URL, the crop mode, the
// reference dimensions, the debug flags.
//
// It is declared here, rather than this package importing config/store,
// because the store depends on the schemas in this package. Importing it back
// would close a cycle (config -> store -> channel -> config). Instead the
// store satisfies this interface and cmd installs it at startup with
// SetCurrent.
type Provider interface {
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	GetFloat64(key string) float64
}

var (
	currentMu sync.RWMutex
	current   Provider
)

// SetCurrent installs the process-wide configuration provider. Called once
// from LoadConfig.
func SetCurrent(p Provider) {
	currentMu.Lock()
	defer currentMu.Unlock()
	current = p
}

// Current returns the process-wide configuration provider.
//
// Before LoadConfig has run -- in a unit test of a package that only reads a
// value or two, say -- this falls back to the schema defaults rather than
// panicking, so reading configuration never needs a test fixture.
func Current() Provider {
	currentMu.RLock()
	defer currentMu.RUnlock()
	if current == nil {
		return defaultProvider{}
	}
	return current
}

// defaultProvider answers from the in-code schema defaults alone.
type defaultProvider struct{}

func (defaultProvider) raw(key string) string {
	d, ok := Lookup(key)
	if !ok {
		logrus.WithField("key", key).Debug(
			"reading a setting that is absent from the schema",
		)
		return ""
	}
	return d.Default
}

func (p defaultProvider) GetString(key string) string { return p.raw(key) }

func (p defaultProvider) GetBool(key string) bool {
	b, _ := strconv.ParseBool(p.raw(key))
	return b
}

func (p defaultProvider) GetInt(key string) int {
	i, _ := strconv.Atoi(p.raw(key))
	return i
}

func (p defaultProvider) GetFloat64(key string) float64 {
	f, _ := strconv.ParseFloat(p.raw(key), 64)
	return f
}

// Convenience readers, so a call site that just wants one value does not have
// to reach through Current itself.

// GetString reads a string setting from the current provider.
func GetString(key string) string { return Current().GetString(key) }

// GetBool reads a boolean setting from the current provider.
func GetBool(key string) bool { return Current().GetBool(key) }

// GetInt reads an integer setting from the current provider.
func GetInt(key string) int { return Current().GetInt(key) }

// GetFloat64 reads a float setting from the current provider.
func GetFloat64(key string) float64 { return Current().GetFloat64(key) }
