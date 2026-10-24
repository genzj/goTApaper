package gui

import "errors"

// errNoAPIServer is reported when the interface is asked to open but the API
// server it would show is not running.
//
// This is not a user error: the daemon starts the API before the tray, so the
// only way to reach it is a failed Start, which has already been logged.
var errNoAPIServer = errors.New(
	"the configuration interface is not available because the API server did not start",
)
