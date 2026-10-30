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
