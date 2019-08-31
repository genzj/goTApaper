// +build dev

package data

import "net/http"

// ExampleAssets include sample config etc
var ExampleAssets http.FileSystem = http.Dir("examples")
