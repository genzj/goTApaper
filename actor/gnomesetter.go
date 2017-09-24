package actor

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/Sirupsen/logrus"
)

func setWithCommand(path string, arg ...string) error {
	cmd := exec.Command(path, arg...)
	logrus.Debugf("executing setter with command %s %v", path, arg)
	if err := cmd.Start(); err != nil {
		return err
	} else {
		return nil
	}
}

type Gnome3Setter int

// SetWallpaper can set wallpaper by gsetting cli tool
func (_ Gnome3Setter) Set(filename string) error {
	path, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	return setWithCommand(
		"gsettings",
		"set",
		"org.gnome.desktop.background",
		"picture-uri",
		fmt.Sprintf("file://%s", path),
	)
}

type Gnome2Setter int

// SetWallpaper can set wallpaper by gsetting cli tool
func (_ Gnome2Setter) Set(filename string) error {
	path, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	return setWithCommand(
		"gconftool-2",
		"--type=string",
		"--set",
		"/desktop/gnome/background/picture_filename",
		path,
	)
}

func init() {
	Setters.Register("gnome2", Gnome2Setter(0))
	Setters.Register("gnome3", Gnome3Setter(0))
}