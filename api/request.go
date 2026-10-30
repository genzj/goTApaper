package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

// Request parsing and file responses shared by every handler.

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
