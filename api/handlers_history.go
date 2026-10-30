package api

import "net/http"

// registerHistoryRoutes serves the wallpaper history: browsing it, previewing
// and re-applying entries, and pruning it.
func (h *handlers) registerHistoryRoutes(r *router) {
	r.handle("GET", "/history", h.listWallpaperHistory)
	r.handle("POST", "/history/prune", h.pruneHistory)
	r.handle("GET", "/history/{id}", h.getWallpaperHistoryEntry)
	r.handle("DELETE", "/history/{id}", h.deleteWallpaperHistoryEntry)
	r.handle("GET", "/history/{id}/thumbnail", h.historyThumbnail)
	r.handle("POST", "/history/{id}/reapply", h.reapplyHistoryEntry)
}

func (h *handlers) listWallpaperHistory(w http.ResponseWriter, r *http.Request) {
	page, err := h.wallpaper.WallpaperHistory(
		intQuery(r, "limit", 0), intQuery(r, "offset", 0),
	)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *handlers) getWallpaperHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	entry, err := h.wallpaper.WallpaperHistoryEntry(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *handlers) deleteWallpaperHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.wallpaper.DeleteWallpaperHistoryEntry(id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *handlers) historyThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	file, err := h.wallpaper.Thumbnail(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	defer file.Close()

	// Thumbnails are immutable once written -- a new wallpaper gets a new
	// entry and a new file -- so they are safe to cache hard, which keeps a
	// scrolling history list from re-fetching every image.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	serveFile(w, r, file, "image/jpeg")
}

func (h *handlers) reapplyHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	h.events.Publish(Event{Type: EventRefreshStarted})
	result, err := h.wallpaper.Reapply(id)
	h.events.Publish(Event{Type: EventRefreshFinished, Data: result})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) pruneHistory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		// KeepAtMost of zero means "use the configured cap".
		KeepAtMost int  `json:"keepAtMost"`
		DryRun     bool `json:"dryRun"`
	}
	if r.ContentLength > 0 && !decodeBody(w, r, &body) {
		return
	}

	result, err := h.wallpaper.PruneHistory(body.KeepAtMost, body.DryRun)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
