//go:build darwin

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void gotapaperSetForeground(int foreground) {
	if (foreground) {
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
		[NSApp activateIgnoringOtherApps:YES];
	} else {
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// setForeground makes the application a regular one while its window is
// open, and an accessory -- tray only -- while it is not.
//
// An accessory application has no Dock icon and is absent from Cmd+Tab, which
// is right for a tray tool with nothing on screen but wrong once its window is
// open: the window could not be switched back to. Wails sets the activation
// policy once, at launch, and offers no way to change it, so it is switched
// here. AppKit requires the main thread.
func setForeground(foreground bool) {
	flag := C.int(0)
	if foreground {
		flag = 1
	}
	application.InvokeSync(func() { C.gotapaperSetForeground(flag) })
}
