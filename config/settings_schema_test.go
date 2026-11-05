package config

import (
	"slices"
	"testing"
)

func TestRetiredKeysAreNotInTheSchema(t *testing.T) {
	for _, key := range []string{"history.file", "history-file", "language"} {
		if !IsRetired(key) {
			t.Errorf("IsRetired(%q) = false, want true", key)
		}
	}
	if IsRetired(KeyLogDebug) {
		t.Errorf("IsRetired(%q) = true for a live setting", KeyLogDebug)
	}
	if got, want := RetiredFlags(), []string{"history-file", "lang"}; !slices.Equal(got, want) {
		t.Errorf("RetiredFlags() = %v, want %v", got, want)
	}

	// A key both retired and defined would be skipped by the import and
	// ignored in the database while still being offered to the user.
	for _, d := range Definitions() {
		for _, key := range append([]string{d.Key}, d.LegacyKeys...) {
			if IsRetired(key) {
				t.Errorf("%q is retired but still spelled by the %s definition", key, d.Key)
			}
		}
	}
}
