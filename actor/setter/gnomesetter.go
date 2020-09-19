// +build linux

package setter

import (
	"fmt"
	"path/filepath"
)

// Gnome3Setter works in gnome 3.x (gsettings)
type Gnome3Setter int

// Set can set wallpaper by gsetting cli tool
func (g Gnome3Setter) Set(filename string) error {
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

// Gnome2Setter works for gnome 2.x (gconftool-2)
type Gnome2Setter int

// Set can set wallpaper by gconftool
func (g Gnome2Setter) Set(filename string) error {
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
