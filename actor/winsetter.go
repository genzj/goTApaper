package actor

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll") //user32.dll
	systemParametersInfo = user32.NewProc("SystemParametersInfoW")
)

type Win32Setter int

// SetWallpaper can set windows wallpaper
func (_ Win32Setter) Set(filename string) error {
	filename, err := filepath.Abs(filename)
	if err != nil {
		return err
	}

	imageLocPtr, err := syscall.UTF16PtrFromString(filename)
	if err != nil {
		return err
	}

	ret, _, _ := systemParametersInfo.Call(
		20,
		0,
		uintptr(unsafe.Pointer(imageLocPtr)), 1)
	if ret != 0 {
		return nil
	}
	return windows.GetLastError()
}
