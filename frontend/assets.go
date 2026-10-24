// Package frontend embeds the built web interface.
//
// It is its own package so that the //go:embed lives next to the directory it
// embeds, and so a build that wants only the CLI can leave it out of the
// import graph entirely.
//
// dist/ is generated: run "make frontend" (or "npm run build" in this
// directory) before building. The committed placeholder keeps this package
// compiling on a fresh checkout, where the real bundle has not been built yet
// -- without it, //go:embed fails and nothing in the repository builds at all.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Assets returns the built interface rooted at dist, ready to serve.
func Assets() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}

// MustAssets returns the built interface, or nil if it cannot be opened.
//
// A nil filesystem is what the API server treats as "this build has no web
// interface", so a missing bundle degrades to a CLI-only binary rather than
// stopping it from starting.
func MustAssets() fs.FS {
	assets, err := Assets()
	if err != nil {
		return nil
	}
	return assets
}
