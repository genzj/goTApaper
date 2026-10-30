package api

import (
	"fmt"
	"net/http"
)

// registerSettingRoutes serves the global scalar settings, and the schema describing them.
func (h *handlers) registerSettingRoutes(r *router) {
	r.handle("GET", "/settings", h.listSettings)
	r.handle("GET", "/settings/{key}", h.getSetting)
	r.handle("PUT", "/settings/{key}", h.putSetting)
	r.handle("DELETE", "/settings/{key}", h.deleteSetting)
	r.handle("GET", "/setting-definitions", h.settingDefinitions)
}

func (h *handlers) listSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.config.Settings()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *handlers) settingDefinitions(w http.ResponseWriter, r *http.Request) {
	// Serving the schema is what lets a client render forms and validate input
	// without shipping its own copy of the vocabularies.
	writeJSON(w, http.StatusOK, h.config.SettingDefinitions())
}

func (h *handlers) getSetting(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	settings, err := h.config.Settings()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	for _, s := range settings {
		if s.Key == key {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	// 400, not 404: the route matched, so it is the key that is wrong.
	writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown setting %q", key))
}

func (h *handlers) putSetting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Value string `json:"value"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := h.config.SetSetting(r.PathValue("key"), body.Value); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *handlers) deleteSetting(w http.ResponseWriter, r *http.Request) {
	if err := h.config.UnsetSetting(r.PathValue("key")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
