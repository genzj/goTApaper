//go:build linux

package setter

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// isSameFile checks if two string paths point to the exact same physical file
func isSameFile(path1, path2 string) bool {
	// 1. Get FileInfo for the first path
	info1, err := os.Stat(path1)
	if err != nil {
		logrus.WithError(err).Warnf("cannot stat file1 %s", path1)
		return false
	}

	// 2. Get FileInfo for the second path
	info2, err := os.Stat(path2)
	if err != nil {
		logrus.WithError(err).Warnf("cannot stat file2 %s", path2)
		return false
	}

	// 3. Compare the two FileInfo objects
	return os.SameFile(info1, info2)
}

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
	); isSameFile(strings.TrimRight(currentWallpaper, "\r\n"), path) {
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
