package api

import "net/http"

// registerWallpaperRoutes serves the wallpaper on the desktop, and asking for a new one.
func (h *handlers) registerWallpaperRoutes(r *router) {
	r.handle("GET", "/wallpaper/current", h.currentWallpaper)
	r.handle("GET", "/wallpaper/current/image", h.currentWallpaperImage)
	r.handle("POST", "/refresh", h.refresh)
}

func (h *handlers) currentWallpaper(w http.ResponseWriter, r *http.Request) {
	current, err := h.wallpaper.Current()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (h *handlers) currentWallpaperImage(w http.ResponseWriter, r *http.Request) {
	file, contentType, err := h.wallpaper.CurrentImage()
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

func (h *handlers) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channels []string `json:"channels"`
	}
	// An empty body means "use the configured rotation", so a decode failure
	// on no content is not an error.
	if r.ContentLength > 0 && !decodeBody(w, r, &body) {
		return
	}

	// Bracketed so a UI that did not start this refresh still sees it happen.
	h.events.Publish(Event{Type: EventRefreshStarted})
	result, err := h.wallpaper.RefreshNow(body.Channels)
	h.events.Publish(Event{Type: EventRefreshFinished, Data: result})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
