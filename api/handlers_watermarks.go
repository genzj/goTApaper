package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/genzj/goTApaper/config/store"
)

// registerWatermarkRoutes serves the watermark layers, the text overlays drawn on each wallpaper.
func (h *handlers) registerWatermarkRoutes(r *router) {
	r.handle("GET", "/watermark-layers", h.listWatermarkLayers)
	r.handle("POST", "/watermark-layers", h.upsertWatermarkLayer)
	r.handle("PUT", "/watermark-layers/{id}", h.upsertWatermarkLayer)
	r.handle("DELETE", "/watermark-layers/{id}", h.deleteWatermarkLayer)
}

func (h *handlers) listWatermarkLayers(w http.ResponseWriter, r *http.Request) {
	layers, err := h.config.WatermarkLayers()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layers)
}

func (h *handlers) upsertWatermarkLayer(w http.ResponseWriter, r *http.Request) {
	var l store.WatermarkLayer
	if !decodeBody(w, r, &l) {
		return
	}
	if raw := r.PathValue("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid layer id %q", raw))
			return
		}
		l.ID = id
	}

	if err := h.config.UpsertWatermarkLayer(l); err != nil {
		writeStoreError(w, err)
		return
	}

	layers, err := h.config.WatermarkLayers()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layers)
}

func (h *handlers) deleteWatermarkLayer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid layer id")
		return
	}
	if err := h.config.DeleteWatermarkLayer(id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
