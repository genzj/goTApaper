package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/service"
)

// registerWatermarkRoutes serves the watermark layers, the text overlays drawn
// on each wallpaper, and the watermark editor: fonts, presets, the template
// reference and the preview.
func (h *handlers) registerWatermarkRoutes(r *router) {
	r.handle("GET", "/watermark-layers", h.listWatermarkLayers)
	r.handle("POST", "/watermark-layers", h.upsertWatermarkLayer)
	r.handle("PUT", "/watermark-layers", h.replaceWatermarkLayers)
	r.handle("PUT", "/watermark-layers/{id}", h.upsertWatermarkLayer)
	r.handle("DELETE", "/watermark-layers/{id}", h.deleteWatermarkLayer)

	r.handle("GET", "/fonts", h.listFonts)
	r.handle("GET", "/fonts/unsupported", h.listUnsupportedFonts)
	r.handle("GET", "/fonts/{id}/file", h.fontFile)
	r.handle("GET", "/watermark-presets", h.watermarkPresets)
	r.handle("GET", "/watermark/template-fields", h.templateFields)

	r.handle("GET", "/watermark/preview/base", h.previewBase)
	r.handle("POST", "/watermark/preview/capture", h.capturePreview)
	r.handle("POST", "/watermark/preview", h.renderPreview)
	r.handle("GET", "/watermark/preview/image", h.previewImage)
	r.handle("POST", "/watermark/preview/sample", h.previewSample)
	r.handle("POST", "/watermark/preview/desktop", h.previewOnDesktop)
	r.handle("POST", "/watermark/preview/revert", h.revertPreview)
}

func (h *handlers) listWatermarkLayers(w http.ResponseWriter, r *http.Request) {
	layers, err := h.config.WatermarkLayers()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(layers))
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
	writeJSON(w, http.StatusOK, nonNil(layers))
}

// replaceWatermarkLayers saves the editor's whole list. The layers apply from
// the next refresh; saving does not change the desktop.
func (h *handlers) replaceWatermarkLayers(w http.ResponseWriter, r *http.Request) {
	var layers []store.WatermarkLayer
	if !decodeBody(w, r, &layers) {
		return
	}
	saved, err := h.config.ReplaceWatermarkLayers(layers)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(saved))
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

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// editor answers 501 when the server was started without the editor.
func (h *handlers) editor(w http.ResponseWriter) bool {
	if h.watermarks == nil {
		writeError(w, http.StatusNotImplemented, "the watermark editor is not available here")
		return false
	}
	return true
}

func (h *handlers) listFonts(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	writeJSON(w, http.StatusOK, nonNil(h.watermarks.Fonts()))
}

// listUnsupportedFonts names the installed fonts the renderer cannot load,
// with the reason, so the editor can explain why one is not offered.
func (h *handlers) listUnsupportedFonts(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	writeJSON(w, http.StatusOK, nonNil(h.watermarks.UnsupportedFonts()))
}

// fontFile serves a font for the editor's font picker to show a sample in.
// Only fonts in the index are served, looked up by id; the request names no
// path, so it cannot reach any other file.
func (h *handlers) fontFile(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	path, err := h.watermarks.FontFile(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeStoreError(w, fmt.Errorf("%w: %v", store.ErrNotFound, err))
		return
	}
	defer file.Close()
	w.Header().Set("Cache-Control", "private, max-age=86400")
	serveFile(w, r, file, "font/ttf")
}

func (h *handlers) watermarkPresets(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	writeJSON(w, http.StatusOK, h.watermarks.Presets())
}

func (h *handlers) templateFields(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	writeJSON(w, http.StatusOK, h.watermarks.TemplateHelp())
}

func (h *handlers) previewBase(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	base, err := h.watermarks.Base()
	if err != nil {
		writePreviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, base)
}

func (h *handlers) capturePreview(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	base, err := h.watermarks.Capture()
	if err != nil {
		writePreviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, base)
}

// renderPreview renders draft layers onto the captured picture and writes the
// preview file. The result reports where each layer landed; the image itself
// is fetched from /watermark/preview/image.
func (h *handlers) renderPreview(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	var body struct {
		Layers []store.WatermarkLayer `json:"layers"`
		// Debug draws the rendering debug overlay. Absent means the
		// debug-rendering setting.
		Debug *bool `json:"debug"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	debug := config.GetBool(config.KeyLogDebugRendering)
	if body.Debug != nil {
		debug = *body.Debug
	}

	res, err := h.watermarks.Render(r.Context(), body.Layers, debug)
	if err != nil {
		writePreviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// previewImage serves the latest render, scaled to the width asked for.
func (h *handlers) previewImage(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	data, revision, err := h.watermarks.Image(intQuery(r, "w", 0))
	if err != nil {
		writePreviewError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	// Never cached: the same URL serves a new render after every change.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Preview-Revision", strconv.FormatInt(revision, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// previewSample renders layers small, without changing the preview, for the
// preset gallery.
func (h *handlers) previewSample(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	var body struct {
		Layers []store.WatermarkLayer `json:"layers"`
		Width  int                    `json:"width"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	data, err := h.watermarks.Sample(body.Layers, body.Width)
	if err != nil {
		writePreviewError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// previewOnDesktop sets the latest render on the desktop, temporarily. It
// answers 202 and runs as a job, refused with 409 while a download runs.
func (h *handlers) previewOnDesktop(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	if _, _, err := h.watermarks.PreviewFile(); err != nil {
		writePreviewError(w, err)
		return
	}
	if err := h.downloader.ShowPreview("api"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.downloader.Status())
}

// revertPreview puts the real wallpaper back on the desktop.
func (h *handlers) revertPreview(w http.ResponseWriter, r *http.Request) {
	if !h.editor(w) {
		return
	}
	if err := h.downloader.RevertPreview("api"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.downloader.Status())
}

// writePreviewError maps the editor's own errors, then defers to the store's.
func writePreviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNoCapture):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrSuperseded):
		// 409: the request was valid, a newer one simply replaced it.
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeStoreError(w, err)
	}
}
