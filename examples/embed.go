// Package examples embeds the example configuration and the tray icons.
//
// It lives beside the files because //go:embed can only reach files in its
// own directory and below. It replaces a vfsgen code generator that wrote a
// git-ignored file, which meant a fresh clone -- or "make clean" -- could not
// be built with plain "go build" until "make generate" had run.
package examples

import "embed"

// FS holds the example configuration and the assets directory.
//
//go:embed config.yaml.example assets
var FS embed.FS
