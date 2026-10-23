package util

import (
	"path/filepath"
	"testing"
)

// TestIsPathUnder pins the containment check that confines deletions to the
// directory they are meant to stay in.
//
// The boundary case is the one that matters: a plain string prefix test would
// report "/tmp/abc" as living under "/tmp/ab", which would let a caller delete
// outside the directory it meant to.
func TestIsPathUnder(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		dir  string
		want bool
	}{
		{name: "direct child", path: "/tmp/ab/file", dir: "/tmp/ab", want: true},
		{name: "deep descendant", path: "/tmp/ab/c/d/file", dir: "/tmp/ab", want: true},
		{name: "the directory itself", path: "/tmp/ab", dir: "/tmp/ab", want: true},
		{name: "needs cleaning first", path: "/tmp/ab/./c/../file", dir: "/tmp/ab", want: true},
		{name: "sibling sharing a prefix", path: "/tmp/abc/file", dir: "/tmp/ab", want: false},
		{name: "parent", path: "/tmp", dir: "/tmp/ab", want: false},
		{name: "escapes upwards", path: "/tmp/ab/../file", dir: "/tmp/ab", want: false},
		{name: "unrelated", path: "/var/file", dir: "/tmp/ab", want: false},
		{name: "empty directory matches nothing", path: "/tmp/ab/file", dir: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPathUnder(tc.path, tc.dir); got != tc.want {
				t.Errorf("IsPathUnder(%q, %q) = %v, want %v", tc.path, tc.dir, got, tc.want)
			}
		})
	}
}

// TestIsPathUnderRelative covers a relative argument, which the deletion
// guards may be handed if a caller has not resolved its paths yet.
func TestIsPathUnderRelative(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if !IsPathUnder("file", dir) {
		t.Errorf("a relative path in %q was reported outside it", dir)
	}
	if IsPathUnder("file", filepath.Join(dir, "sub")) {
		t.Error("a relative path was reported inside a subdirectory it is not in")
	}
}
