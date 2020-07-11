// +build darwin

package actor

import "strconv"

type DarwinSetter int

// Set set the desktop wallpaper to specified file by osascript
func (_ DarwinSetter) Set(file string) error {
	return setWithCommand(
		"osascript",
		"-e",
		`tell application "System Events" to tell every desktop to set picture to `+strconv.Quote(file),
	)
}

func init() {
	Setters.Register("darwin", DarwinSetter(0))
}
