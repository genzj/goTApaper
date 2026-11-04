// Package data serves the files embedded from examples/.
package data

import (
	"net/http"

	"github.com/genzj/goTApaper/examples"
)

// ExampleAssets includes the sample configuration and the tray icons.
//
// An http.FileSystem, as it was when vfsgen generated it, so callers that open
// "/assets/<name>" or "config.yaml.example" are unchanged: http.FS strips the
// leading slash.
var ExampleAssets http.FileSystem = http.FS(examples.FS)
