package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/api/wire"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/service"
)

type handlers struct {
	config    *service.Config
	wallpaper *service.Wallpaper
	logs      *service.Logs
	events    *eventHub
}

// errorBody is the single error shape every failing route returns. It is
// shared with clients via the wire package.
type errorBody = wire.ErrorBody

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logrus.WithError(err).Debug("cannot write the API response")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorBody{Error: message})
}

// writeStoreError maps a store or validation error onto a status code, so a
// client can tell "you asked for something missing" from "you sent something
// invalid" from "that would break other things".
func writeStoreError(w http.ResponseWriter, err error) {
	var invalid *config.InvalidValueError

	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, store.ErrTemplateInUse):
		// 409, because the caller has a real choice to make: reassign the
		// channels, or inline the template. The dependants are named in the
		// message, and deleteTemplate adds them as a structured field.
		writeError(w, http.StatusConflict, err.Error())

	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, err.Error())

	case errors.Is(err, config.ErrUnknownKey):
		// 400 rather than 404: the route exists and was matched, so the
		// problem is the key the caller supplied. Reserving 404 for an
		// unrouted path keeps "you called the wrong endpoint" distinguishable
		// from "you named a setting that does not exist".
		writeError(w, http.StatusBadRequest, err.Error())

	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, errorBody{
			Error: invalid.Error(), Expected: invalid.Expected,
		})

	default:
		logrus.WithError(err).Error("API request failed")
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	// Reject unknown fields: silently ignoring a misspelled one would look
	// like the write succeeded while doing nothing.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot decode the request body: %v", err))
		return false
	}
	return true
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// settings

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

// channels

func (h *handlers) listChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.config.Channels()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (h *handlers) getChannel(w http.ResponseWriter, r *http.Request) {
	c, err := h.config.Channel(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) upsertChannel(w http.ResponseWriter, r *http.Request) {
	var c store.ChannelInstance
	if !decodeBody(w, r, &c) {
		return
	}
	// On PUT the path is authoritative, so a mismatched body cannot rename a
	// channel by accident.
	if name := r.PathValue("name"); name != "" {
		c.Name = name
	}

	if err := h.config.UpsertChannel(c); err != nil {
		writeStoreError(w, err)
		return
	}

	updated, err := h.config.Channel(c.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *handlers) deleteChannel(w http.ResponseWriter, r *http.Request) {
	if err := h.config.DeleteChannel(r.PathValue("name")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *handlers) reorderChannels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Names []string `json:"names"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := h.config.ReorderChannels(body.Names); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *handlers) channelTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.config.ChannelTypes())
}

// channel templates

func (h *handlers) listTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.config.ChannelTemplates()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

func (h *handlers) getTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := h.config.ChannelTemplate(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *handlers) upsertTemplate(w http.ResponseWriter, r *http.Request) {
	var t store.ChannelTemplate
	if !decodeBody(w, r, &t) {
		return
	}
	if name := r.PathValue("name"); name != "" {
		t.Name = name
	}
	if err := h.config.UpsertChannelTemplate(t); err != nil {
		writeStoreError(w, err)
		return
	}

	updated, err := h.config.ChannelTemplate(t.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *handlers) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Fetch the dependants first, so a refusal can name them rather than
	// leaving the client to parse the message.
	view, viewErr := h.config.ChannelTemplate(name)

	if err := h.config.DeleteChannelTemplate(name); err != nil {
		if errors.Is(err, store.ErrTemplateInUse) && viewErr == nil {
			writeJSON(w, http.StatusConflict, errorBody{
				Error:  err.Error(),
				UsedBy: view.UsedBy,
			})
			return
		}
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *handlers) inlineTemplate(w http.ResponseWriter, r *http.Request) {
	if err := h.config.InlineChannelTemplate(r.PathValue("name")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// watermark layers

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

// wallpaper

// listLatestPerChannel serves the deduplication pointer: one row per channel,
// overwritten on every download. Distinct from the browsable history below,
// which is a log.
func (h *handlers) listLatestPerChannel(w http.ResponseWriter, r *http.Request) {
	entries, err := h.wallpaper.History()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
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

// logs

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

// pathID parses an {id} path segment, answering the request itself on failure.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid id %q", raw))
		return 0, false
	}
	return id, true
}

// intQuery reads an integer query parameter, falling back to def when it is
// absent or unparseable.
//
// Unparseable is tolerated rather than rejected because these are display
// hints -- a page size, an offset -- and a client sending "?limit=abc" is
// better served a default page than an error it cannot act on. The id in a
// path is validated strictly above, because there the value *is* the request.
func intQuery(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return parsed
}

// serveFile streams an open file, using http.ServeContent so range requests
// and conditional gets work without this package implementing either.
func serveFile(w http.ResponseWriter, r *http.Request, file *os.File, contentType string) {
	w.Header().Set("Content-Type", contentType)

	info, err := file.Stat()
	if err != nil {
		// No size and no modification time: fall back to a plain copy, which
		// loses range support but still delivers the bytes.
		if _, err := io.Copy(w, file); err != nil {
			logrus.WithError(err).Debug("cannot write the file response")
		}
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
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
