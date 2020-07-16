// +build darwin

package actor

import (
	"strconv"
)

type DarwinSetter int

// Set set the desktop wallpaper to specified file by osascript
func (_ DarwinSetter) Set(file string) error {
	err := setWithCommand(
		"osascript",
		"-e",
		`tell application "System Events" to tell every desktop to set picture to `+strconv.Quote(file),
	)
	if err != nil {
		return err
	}

	// WA: force Dock to reload the overwritten picture
	return setWithCommand("killall", "Dock")
}

func init() {
	Setters.Register("darwin", DarwinSetter(0))
}
