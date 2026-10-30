package api

import (
	"net/http"

	"github.com/genzj/goTApaper/service"
)

// registerWallpaperRoutes serves the wallpaper goTApaper applied last, and
// downloading a new one.
func (h *handlers) registerWallpaperRoutes(r *router) {
	r.handle("GET", "/wallpaper/last-applied", h.lastApplied)
	r.handle("GET", "/wallpaper/last-applied/image", h.lastAppliedImage)
	r.handle("POST", "/refresh", h.refresh)
	r.handle("GET", "/status", h.status)
}

func (h *handlers) lastApplied(w http.ResponseWriter, r *http.Request) {
	last, err := h.wallpaper.LastApplied()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, last)
}

func (h *handlers) lastAppliedImage(w http.ResponseWriter, r *http.Request) {
	file, contentType, err := h.wallpaper.LastAppliedImage()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	defer file.Close()

	// Deliberately not cached: this path keeps serving a different picture as
	// refreshes replace the file underneath it.
	w.Header().Set("Cache-Control", "no-store")
	serveFile(w, r, file, contentType)
}

// refresh starts a refresh and answers at once. The download happens in the
// background, and its end arrives as a download-finished event; waiting for it
// here would hold the request open for as long as the provider takes to
// serve a multi-megabyte picture.
func (h *handlers) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channels []string `json:"channels"`
		// Force downloads even a picture the channel applied last time.
		Force bool `json:"force"`
	}
	// An empty body means "use the configured rotation", so a decode failure
	// on no content is not an error.
	if r.ContentLength > 0 && !decodeBody(w, r, &body) {
		return
	}

	if err := h.downloader.Refresh(body.Channels, body.Force); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.downloader.Status())
}

// StatusBody is what GET /status returns.
type StatusBody struct {
	// Version is the running build, so a client can tell which daemon it is
	// talking to.
	Version  string                 `json:"version,omitempty"`
	Download service.DownloadStatus `json:"download"`
}

func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, StatusBody{
		Version:  h.version,
		Download: h.downloader.Status(),
	})
}
