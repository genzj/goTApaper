//go:build linux

package setter

import (
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// DMSSetter works in Dank Material Shell (DMS)
type DMSSetter int

// Set can set wallpaper by gsetting cli tool
func (g DMSSetter) Set(filename string) error {
	path, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	if currentWallpaper, _ := runCommand(
		"dms",
		"ipc",
		"call",
		"wallpaper",
		"get",
	); samePathOrFile(strings.TrimRight(currentWallpaper, "\r\n"), path) {
		logrus.Debugf("force reloading wallpaper by clearing it first.")
		runCommand(
			"dms",
			"ipc",
			"call",
			"wallpaper",
			"clear",
		)
	}
	return setWithCommand(
		"dms",
		"ipc",
		"call",
		"wallpaper",
		"set",
		path,
	)
}

func init() {
	Setters.Register("dunkmaterialshell", DMSSetter(0))
	Setters.Register("dms", DMSSetter(0))
}
