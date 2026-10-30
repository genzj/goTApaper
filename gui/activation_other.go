//go:build !darwin

package gui

// setForeground does nothing outside macOS: Windows and Linux list a window in
// the task switcher whether or not the application also has a tray icon.
func setForeground(bool) {}
