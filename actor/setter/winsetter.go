// +build windows

package setter

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"

	"golang.org/x/sys/windows"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll") //user32.dll
	systemParametersInfo = user32.NewProc("SystemParametersInfoW")
)

// Win32Setter supports Windows 7~10
type Win32Setter int

// Set windows wallpaper
func (Win32Setter) Set(filename string) error {
	filename, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	logrus.Debugf("setting wallpaper %s", filename)

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

func init() {
	Setters.Register("windows", Win32Setter(0))
}
