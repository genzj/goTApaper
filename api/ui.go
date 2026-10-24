package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// uiAssets holds the built frontend, installed by the application at startup.
//
// It lives here rather than being embedded in this package because the build
// that produces frontend/dist is driven from the application's main package,
// and a //go:embed of a directory that may not exist yet would stop this
// package compiling on a checkout where the frontend has not been built.
var uiAssets fs.FS

// SetUIAssets installs the built frontend to serve under /ui/.
//
// Called before Start. With no assets installed the /ui/ routes report that
// the interface is unavailable, which is the right answer for a build that
// deliberately ships only the CLI.
func SetUIAssets(assets fs.FS) { uiAssets = assets }

// uiHandler serves the single-page frontend.
type uiHandler struct {
	assets fs.FS
}

func (u *uiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if u.assets == nil {
		writeError(w, http.StatusNotFound,
			"this build does not include the web interface")
		return
	}

	name := strings.TrimPrefix(r.URL.Path, uiPath)
	if name == "" {
		name = "index.html"
	}

	file, err := u.assets.Open(name)
	if err != nil {
		// A single-page application owns its own routing, so an unknown path
		// is a client-side route rather than a missing file: serve the
		// document and let the app resolve it. Asset requests are
		// distinguished by having an extension, so a genuinely missing
		// stylesheet still reports 404 instead of quietly returning HTML.
		if strings.Contains(name, ".") {
			writeError(w, http.StatusNotFound, "no such asset: "+name)
			return
		}
		name = "index.html"
		if file, err = u.assets.Open(name); err != nil {
			writeError(w, http.StatusNotFound, "the web interface is not available")
			return
		}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "no such asset: "+name)
		return
	}

	// The hashed bundles Vite emits are safe to cache forever; index.html is
	// what points at the current ones, so it must not be.
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	}

	seeker, ok := file.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		logrus.WithField("asset", name).Debug("asset is not seekable")
		writeError(w, http.StatusInternalServerError, "cannot serve "+name)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
}
