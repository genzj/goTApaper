package cmd

import (
	"strings"
	"testing"
)

func TestInstanceIDIsPerApplicationDirectory(t *testing.T) {
	a := instanceID("/home/u/.goTApaper")
	b := instanceID("/tmp/throwaway")

	// One interface per directory, not one per machine: two databases can
	// each have their own writer.
	if a == b {
		t.Fatal("two directories share an instance ID")
	}
	if a != instanceID("/home/u/.goTApaper") {
		t.Fatal("the instance ID is not stable for one directory")
	}
	// The path itself must not leak into the lock file name.
	if strings.Contains(a, "home") || strings.Contains(a, "/") {
		t.Fatalf("instance ID %q exposes the path", a)
	}
}

func TestInstanceIDIsValidDBusName(t *testing.T) {
	// "/home/genzj/.goTApaper" hashes to 0900c44755cc1d14..., whose leading
	// digit would be rejected by D-Bus if used directly as an element.
	dirs := []string{
		"/home/genzj/.goTApaper",
		"/home/u/.goTApaper",
		"/tmp/throwaway",
		"",
	}
	for _, dir := range dirs {
		id := instanceID(dir)
		parts := strings.Split(id, ".")
		if len(parts) < 2 {
			t.Fatalf("instance ID %q has fewer than two dot-separated elements", id)
		}
		for _, part := range parts {
			if part == "" {
				t.Fatalf("instance ID %q has an empty element", id)
			}
			first := part[0]
			if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
				t.Fatalf("instance ID %q for %q has element %q not starting with an ASCII letter", id, dir, part)
			}
		}
	}
}
