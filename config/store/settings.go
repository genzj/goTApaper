package store

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// Options implements channel.Settings over a resolved channel's option map.
//
// It is the bridge between the store and the download pipeline: channels read
// their configuration through a handful of typed getters, and this supplies
// them from an EffectiveChannel's already-merged options.
type Options struct {
	values map[string]any
}

// NewOptions wraps an option map. The map is not copied, so callers should not
// mutate it afterwards.
func NewOptions(values map[string]any) *Options {
	if values == nil {
		values = map[string]any{}
	}
	return &Options{values: values}
}

// Settings returns a channel.Settings view of this channel's options, with any
// per-refresh overrides applied on top.
//
// Overrides exist for values that are decided per run rather than stored --
// "force" being the only one today -- so they never touch the database.
func (c EffectiveChannel) Settings(overrides map[string]any) *Options {
	merged := make(map[string]any, len(c.Options)+len(overrides))
	for k, v := range c.Options {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return NewOptions(merged)
}

// lookup resolves a key, descending into nested maps for a dotted key such as
// "meta.title". The flat key is tried first so an option literally named
// "meta.title" still wins, which is how it arrives from the YAML import.
func (o *Options) lookup(key string) (any, bool) {
	if v, ok := o.values[key]; ok {
		return v, true
	}

	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		return nil, false
	}

	var current any = o.values
	for _, part := range parts {
		m, ok := asStringKeyMap(current)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// asStringKeyMap normalises the two map shapes an option tree can hold. JSON
// decoding yields map[string]any, while a YAML document can yield map[any]any,
// and both reach this code.
func asStringKeyMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}

// IsSet reports whether a key has a value.
func (o *Options) IsSet(key string) bool {
	_, ok := o.lookup(key)
	return ok
}

// GetString returns a key as a string, empty if absent.
//
// Numbers are formatted rather than rejected: several channels read width and
// dpr as strings to splice straight into a query, and a value stored as JSON
// arrives here as a float64.
func (o *Options) GetString(key string) string {
	v, ok := o.lookup(key)
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		// 'f' with precision -1 keeps 1920 as "1920" rather than "1.92e+03".
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		logUnconvertible(key, v, "string")
		return ""
	}
}

// logUnconvertible records an option whose stored type cannot be read as the
// kind a channel asked for.
//
// These getters cannot return an error -- a channel reads its settings as
// plain values -- so without a log the only symptom of a mistyped option is a
// download that quietly behaves as though the option were unset. The value
// itself is logged by type and not by content, since options can hold API
// keys.
func logUnconvertible(key string, value any, want string) {
	logrus.WithFields(logrus.Fields{
		"option": key,
		"stored": typeName(value),
		"wanted": want,
	}).Warn("a channel option cannot be read as the expected type; treating it as unset")
}

func typeName(v any) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%T", v)
}

// GetBool returns a key as a bool, false if absent or unparseable.
func (o *Options) GetBool(key string) bool {
	v, ok := o.lookup(key)
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, err := strconv.ParseBool(t)
		return err == nil && b
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		logUnconvertible(key, v, "bool")
		return false
	}
}

// GetInt returns a key as an int, zero if absent or unparseable.
func (o *Options) GetInt(key string) int {
	v, ok := o.lookup(key)
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		i, err := strconv.Atoi(t)
		if err != nil {
			logUnconvertible(key, v, "int")
			return 0
		}
		return i
	default:
		logUnconvertible(key, v, "int")
		return 0
	}
}

// GetStringMapString returns a nested map with every value stringified, empty
// if the key is absent or does not hold a map. Used for pass-through query
// parameters such as unsplash's image_parameters.
func (o *Options) GetStringMapString(key string) map[string]string {
	v, ok := o.lookup(key)
	if !ok {
		return map[string]string{}
	}
	m, ok := asStringKeyMap(v)
	if !ok {
		logUnconvertible(key, v, "string map")
		return map[string]string{}
	}

	out := make(map[string]string, len(m))
	nested := &Options{values: m}
	for k := range m {
		out[k] = nested.GetString(k)
	}
	return out
}
