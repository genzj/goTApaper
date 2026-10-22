//go:build !windows

package config

// platformAppDir has no separate location on Unix-like systems: the dotfile
// directory in the user's home is the convention, and is what AppDir already
// falls back to.
//
// XDG_CONFIG_HOME would be the stricter choice on Linux, but moving an
// existing user's configuration is a migration this change does not need to
// take on.
func platformAppDir() string { return "" }
