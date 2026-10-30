//go:build windows

package util

import (
	"os"
	"os/exec"
)

// Reexec starts a fresh copy of the running process, same arguments and
// environment, and returns so the caller can exit.
//
// Windows has no exec, so the copy is a new process with a new PID. The
// caller must have released anything the copy needs -- the single-instance
// lock, the listening port -- before calling this, because both processes
// exist for a moment.
func Reexec() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Start()
}
