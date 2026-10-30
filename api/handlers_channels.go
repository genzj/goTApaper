package api

import (
	"errors"
	"net/http"

	"github.com/genzj/goTApaper/config/store"
)

// registerChannelRoutes serves channels, their types, and the templates they inherit from.
func (h *handlers) registerChannelRoutes(r *router) {
	r.handle("GET", "/channels", h.listChannels)
	r.handle("POST", "/channels", h.upsertChannel)
	r.handle("POST", "/channels/reorder", h.reorderChannels)
	r.handle("GET", "/channels/{name}", h.getChannel)
	r.handle("PUT", "/channels/{name}", h.upsertChannel)
	r.handle("DELETE", "/channels/{name}", h.deleteChannel)
	r.handle("GET", "/channel-types", h.channelTypes)

	r.handle("GET", "/channel-templates", h.listTemplates)
	r.handle("POST", "/channel-templates", h.upsertTemplate)
	r.handle("GET", "/channel-templates/{name}", h.getTemplate)
	r.handle("PUT", "/channel-templates/{name}", h.upsertTemplate)
	r.handle("DELETE", "/channel-templates/{name}", h.deleteTemplate)
	r.handle("POST", "/channel-templates/{name}/inline", h.inlineTemplate)
}

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
