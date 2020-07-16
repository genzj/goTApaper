// +build darwin

package actor

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
)

type DarwinSetter int

// Set set the desktop wallpaper to specified file by osascript
func (_ DarwinSetter) Set(file string) error {
	// MacOS Dock doesn't reload the wallpaper picture if its path is identical
	// to current wallpaper's path, although the content of picture file has
	// been overwritten
	// A popular workaround is restart Dock by shell command "killall Dock". Howeverit has appreciable side-effect. For example if user is using the Misson Control during Dock restarting, Mission Control will be aborted.
	// Here I use another solution that is using a unqiue wallpaper name each
	// time by copying wallpaper picture to a file suffixed with timestamp. Old
	// wallpaper files will be removed after a successful wallpaper setting.
	dir, filename := filepath.Split(file)

	if !strings.HasSuffix(dir, "/") {
		dir = dir + "/"
	}

	wallpaperPath := fmt.Sprintf("%swallpaper_osx_%d%s", dir, time.Now().Unix(), filepath.Ext(filename))
	_, err := util.CopyFile(file, wallpaperPath)
	if err != nil {
		return err
	}

	err = setWithCommand(
		"osascript",
		"-e",
		`tell application "System Events" to tell every desktop to set picture to `+strconv.Quote(wallpaperPath),
	)
	if err != nil {
		return err
	}

	if cleanErr := util.RemoveFilesByGlob(
		dir+"wallpaper_osx_*.*",
		func(path string) bool {
			return path != wallpaperPath
		},
	); cleanErr != nil {
		logrus.Warnf("error during cleaning up old wallpapers: %s", err)
	}
	return err
}

func init() {
	Setters.Register("darwin", DarwinSetter(0))
}
