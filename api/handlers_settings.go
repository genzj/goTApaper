package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// settingValue accepts a JSON string, number, or boolean and normalises it to
// the string form that config.Validate and store.SetSetting expect.
//
// Accepting scalar JSON types directly means a client -- whether curl or a
// form control -- can send {"value": 1800} or {"value": "1800"} for an
// integer setting and {"value": true} or {"value": "true"} for a boolean one.
type settingValue string

func (v *settingValue) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return errors.New("setting value cannot be null")
	}

	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*v = settingValue(s)
		return nil
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(trimmed, &b); err != nil {
			return err
		}
		*v = settingValue(strconv.FormatBool(b))
		return nil
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		var n json.Number
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&n); err != nil {
			return err
		}
		*v = settingValue(n.String())
		return nil
	default:
		return errors.New("setting value must be a string, number, or boolean")
	}
}

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
		Value *settingValue `json:"value"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Value == nil {
		writeError(w, http.StatusBadRequest, `missing required field "value"`)
		return
	}
	if err := h.config.SetSetting(r.PathValue("key"), string(*body.Value)); err != nil {
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
