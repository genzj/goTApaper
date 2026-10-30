package api

import (
	"fmt"
	"net/http"
	"strconv"
)

// registerLogRoutes serves the recent log output, in memory and on disk.
func (h *handlers) registerLogRoutes(r *router) {
	r.handle("GET", "/logs", h.readLogs)
	r.handle("GET", "/logs/files", h.listLogFiles)
	r.handle("GET", "/logs/files/{name}", h.downloadLogFile)
}

func (h *handlers) readLogs(w http.ResponseWriter, r *http.Request) {
	var since uint64
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid since %q", raw))
			return
		}
		since = parsed
	}

	writeJSON(w, http.StatusOK, h.logs.Read(
		r.URL.Query().Get("level"), since, intQuery(r, "limit", 0),
	))
}

func (h *handlers) listLogFiles(w http.ResponseWriter, r *http.Request) {
	files, err := h.logs.Files()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *handlers) downloadLogFile(w http.ResponseWriter, r *http.Request) {
	file, err := h.logs.OpenFile(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Cache-Control", "no-store")
	serveFile(w, r, file, "application/x-ndjson")
}
