package config

import (
	"fmt"
	"slices"
	"strconv"
)

// Kind classifies the type of a configurable value. The database stores every
// value as TEXT; the kind is what tells the store how to parse it and tells a
// UI which control to render.
type Kind string

// Value kinds.
const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindFloat  Kind = "float"
	KindBool   Kind = "bool"
	KindEnum   Kind = "enum"
	// KindStringMap is a free-form string-to-string map, used for
	// pass-through query parameters.
	KindStringMap Kind = "stringMap"
)

// ValueSpec describes one configurable value: its type, its default, and for
// enums the set of values it accepts.
//
// One type covers both global settings and per-channel options. They had
// separate near-identical definitions at first, which meant two places to
// extend when adding a field and two validators that could drift apart on what
// counts as a valid enum. A channel option carries a Label and may be
// Required; a global setting simply leaves those unset.
type ValueSpec struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Kind  Kind   `json:"kind"`
	// Required marks an option a channel cannot run without.
	Required    bool     `json:"required,omitempty"`
	Default     string   `json:"default,omitempty"`
	EnumValues  []string `json:"enumValues,omitempty"`
	Description string   `json:"description,omitempty"`
	// Secret marks a value that must not be echoed back to a client. API keys
	// are redacted on read so they stay out of UI state and request logs.
	Secret bool `json:"secret,omitempty"`
	// LegacyKeys are names this value was known by in an older configuration
	// format, consulted when importing. Never written.
	LegacyKeys []string `json:"-"`
}

// InvalidValueError reports a value that does not fit its spec. It lists the
// acceptable values so the CLI and the API can tell the user what to do
// instead of merely rejecting the input.
type InvalidValueError struct {
	Key      string
	Value    string
	Kind     Kind
	Expected []string
}

func (e *InvalidValueError) Error() string {
	if len(e.Expected) > 0 {
		return fmt.Sprintf(
			"invalid value %q for %s: expected one of %v", e.Value, e.Key, e.Expected,
		)
	}
	return fmt.Sprintf("invalid value %q for %s: expected a %s", e.Value, e.Key, e.Kind)
}

// ValidateValue checks a raw string value against this spec.
func (s ValueSpec) ValidateValue(value string) error {
	invalid := func() error {
		return &InvalidValueError{
			Key: s.Key, Value: value, Kind: s.Kind, Expected: s.EnumValues,
		}
	}

	switch s.Kind {
	case KindString, KindStringMap:
		// A string map arrives already structured; there is no scalar form to
		// check here.
		return nil
	case KindBool:
		if _, err := strconv.ParseBool(value); err != nil {
			return invalid()
		}
	case KindInt:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return invalid()
		}
	case KindFloat:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return invalid()
		}
	case KindEnum:
		if !slices.Contains(s.EnumValues, value) {
			return invalid()
		}
	default:
		return fmt.Errorf("%s has unsupported kind %q", s.Key, s.Kind)
	}
	return nil
}
