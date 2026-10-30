package channel

import "testing"

// mapSettings is the smallest Settings a channel can be handed, which is the
// point of passing the last applied URL through it: the dedup decision needs
// no store.
type mapSettings map[string]any

func (m mapSettings) GetString(key string) string {
	s, _ := m[key].(string)
	return s
}

func (m mapSettings) GetBool(key string) bool {
	b, _ := m[key].(bool)
	return b
}

func (m mapSettings) GetInt(key string) int {
	i, _ := m[key].(int)
	return i
}

func (m mapSettings) GetStringMapString(string) map[string]string { return nil }

func (m mapSettings) IsSet(key string) bool {
	_, ok := m[key]
	return ok
}

func TestAlreadyApplied(t *testing.T) {
	const url = "https://example.invalid/a.jpg"

	for _, tc := range []struct {
		name     string
		settings mapSettings
		url      string
		want     bool
	}{
		{"same picture as last time", mapSettings{OptionKeyLastAppliedURL: url}, url, true},
		{"a different picture", mapSettings{OptionKeyLastAppliedURL: url}, url + "?v=2", false},
		{"no history yet", mapSettings{}, url, false},
		{
			"forced", mapSettings{OptionKeyLastAppliedURL: url, OptionKeyForce: true}, url, false,
		},
		// An empty URL never matches, or a channel that failed to resolve one
		// would look like it had nothing new to download.
		{"no URL resolved", mapSettings{OptionKeyLastAppliedURL: ""}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := alreadyApplied(tc.settings, tc.url); got != tc.want {
				t.Errorf("alreadyApplied = %v, want %v", got, tc.want)
			}
		})
	}
}
