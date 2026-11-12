package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/api/wire"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/service"
)

type handlers struct {
	config     *service.Config
	wallpaper  *service.Wallpaper
	downloader *service.Downloader
	logs       *service.Logs
	events     *eventHub
	lifecycle  Lifecycle
	version    string
}

// errorBody is the single error shape every failing route returns. It is
// shared with clients via the wire package.
type errorBody = wire.ErrorBody

func writeJSON(w http.ResponseWriter, status int, body any) {
	if eb, ok := body.(errorBody); ok && eb.Error != "" &&
		status >= http.StatusBadRequest &&
		status != http.StatusUnauthorized &&
		status != http.StatusForbidden &&
		status != http.StatusInternalServerError {
		logrus.WithFields(logrus.Fields{
			"status": status,
			"error":  eb.Error,
		}).Warn("API request rejected")
	}

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

	case errors.Is(err, service.ErrBusy):
		// 409 rather than 429 or 503: nothing is wrong with the server or the
		// rate of requests, the request conflicts with the download already
		// under way. The client can retry once download-finished arrives.
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

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// channel templates

// watermark layers
