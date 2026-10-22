//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// platformAppDir returns %APPDATA%\<AppName>.
//
// Windows has no dotfile convention, and a directory called ".goTApaper" in
// the user profile is both invisible to Explorer and outside the location
// Windows actually backs up and roams. APPDATA is where per-user application
// configuration belongs; the legacy path is still honoured when it exists, so
// an existing install is not stranded.
func platformAppDir() string {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, getAppName())
	}
	// No APPDATA (a stripped service account, say): fall back to the legacy
	// dotfile directory rather than guessing.
	return ""
}
