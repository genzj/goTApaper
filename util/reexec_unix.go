//go:build !windows

package util

import (
	"os"
	"syscall"
)

// Reexec replaces the running process with a fresh copy of itself, same
// arguments and environment, keeping its PID.
//
// Keeping the PID is what makes a restart transparent to whatever started the
// process: a terminal stays attached, and a service manager such as systemd
// sees one long-running process rather than one that exited. Every file Go
// opens is close-on-exec, so locks held through them -- the GUI's
// single-instance lock, the listening socket -- are released as the new image
// starts.
//
// It returns only on failure.
func Reexec() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
