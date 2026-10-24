// Package client talks to a running goTApaper daemon's API.
//
// It is separate from the server package so that a caller wanting to send one
// request does not pull in every request handler, the service layer and the
// store along with it.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/genzj/goTApaper/api/wire"
)

// requestTimeout bounds a call to the daemon. Generous enough for a refresh to
// be queued, short enough that a wedged daemon does not hang the CLI.
const requestTimeout = 10 * time.Second

// downloadTimeout bounds a call that fetches a picture from a provider before
// it can answer, which no ten-second deadline will cover on a slow link.
const downloadTimeout = 2 * time.Minute

// Client issues requests to a running daemon.
//
// Routing writes through the daemon rather than straight to the database means
// the daemon can act on a change immediately instead of waiting out its sleep
// interval, and the database keeps a single writer.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// Discover returns a client for the daemon described by the handshake files in
// appDir, or nil if no daemon appears to be running.
//
// A leftover endpoint file from a crashed daemon is treated as "no daemon":
// the health check is what distinguishes a live listener from stale state.
func Discover(appDir string) *Client {
	addr, err := os.ReadFile(wire.EndpointPath(appDir))
	if err != nil {
		return nil
	}
	token, err := os.ReadFile(wire.TokenPath(appDir))
	if err != nil {
		return nil
	}

	c := &Client{
		base:  "http://" + strings.TrimSpace(string(addr)),
		token: strings.TrimSpace(string(token)),
		http:  &http.Client{Timeout: requestTimeout},
	}
	if !c.alive() {
		return nil
	}
	return c
}

// alive reports whether the recorded endpoint answers.
func (c *Client) alive() bool {
	resp, err := c.http.Get(c.base + wire.BasePath + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// Addr reports the daemon's address, for logging.
func (c *Client) Addr() string { return strings.TrimPrefix(c.base, "http://") }

// Do issues a request against a path relative to the API base, decoding a JSON
// response into out, which may be nil.
func (c *Client) Do(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, c.base+wire.BasePath+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return decodeError(method, path, resp.Status, payload)
	}

	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

// decodeError prefers the server's own message, which already names the
// accepted values or the blocking dependants, over a bare status line.
func decodeError(method, path, status string, payload []byte) error {
	var e wire.ErrorBody
	if json.Unmarshal(payload, &e) == nil && e.Error != "" {
		if len(e.UsedBy) > 0 {
			return fmt.Errorf("%s (used by %s)", e.Error, strings.Join(e.UsedBy, ", "))
		}
		return fmt.Errorf("%s", e.Error)
	}
	return fmt.Errorf("%s %s: %s", method, path, status)
}

// SetSetting changes one setting through the daemon.
func (c *Client) SetSetting(key, value string) error {
	return c.Do(http.MethodPut, "/settings/"+key,
		map[string]string{"value": value}, nil)
}

// UnsetSetting reverts one setting to its default through the daemon.
func (c *Client) UnsetSetting(key string) error {
	return c.Do(http.MethodDelete, "/settings/"+key, nil, nil)
}

// Wallpaper and history.

// Refresh asks the daemon to download and apply a wallpaper now. An empty
// channel list uses the configured rotation.
func (c *Client) Refresh(channels []string, out any) error {
	return c.Do(http.MethodPost, "/refresh",
		map[string][]string{"channels": channels}, out)
}

// WallpaperHistory reads one page of the browsable history.
func (c *Client) WallpaperHistory(limit, offset int, out any) error {
	return c.Do(http.MethodGet,
		fmt.Sprintf("/history?limit=%d&offset=%d", limit, offset), nil, out)
}

// WallpaperHistoryEntry reads one history entry.
func (c *Client) WallpaperHistoryEntry(id int64, out any) error {
	return c.Do(http.MethodGet, fmt.Sprintf("/history/%d", id), nil, out)
}

// DeleteWallpaperHistoryEntry removes one history entry and its thumbnail.
func (c *Client) DeleteWallpaperHistoryEntry(id int64) error {
	return c.Do(http.MethodDelete, fmt.Sprintf("/history/%d", id), nil, nil)
}

// Reapply downloads a recorded wallpaper again and sets it.
//
// This one needs a longer deadline than the shared timeout allows: it makes an
// outbound HTTP request to the original provider before it can answer.
func (c *Client) Reapply(id int64, out any) error {
	return c.withTimeout(downloadTimeout).Do(
		http.MethodPost, fmt.Sprintf("/history/%d/reapply", id), nil, out,
	)
}

// PruneHistory enforces the history cap through the daemon. A keepAtMost of
// zero means "use the configured cap".
func (c *Client) PruneHistory(keepAtMost int, dryRun bool, out any) error {
	return c.Do(http.MethodPost, "/history/prune", map[string]any{
		"keepAtMost": keepAtMost,
		"dryRun":     dryRun,
	}, out)
}

// CurrentWallpaper reads what is on the desktop now.
func (c *Client) CurrentWallpaper(out any) error {
	return c.Do(http.MethodGet, "/wallpaper/current", nil, out)
}

// Logs.

// Logs reads buffered log entries.
func (c *Client) Logs(level string, since uint64, limit int, out any) error {
	query := url.Values{}
	if level != "" {
		query.Set("level", level)
	}
	if since > 0 {
		query.Set("since", strconv.FormatUint(since, 10))
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	return c.Do(http.MethodGet, "/logs?"+query.Encode(), nil, out)
}

// LogFiles lists the log files on disk.
func (c *Client) LogFiles(out any) error {
	return c.Do(http.MethodGet, "/logs/files", nil, out)
}

// Events opens the server-sent event stream.
//
// It returns the response body for the caller to read and close. No timeout is
// applied: the whole point of the stream is to stay open, and the shared
// client's deadline would cut it off mid-tail.
func (c *Client) Events() (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+wire.BasePath+"/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		return nil, decodeError(http.MethodGet, "/events", resp.Status, payload)
	}
	return resp.Body, nil
}

// withTimeout returns a shallow copy of the client using a different deadline,
// so one slow call does not require relaxing it for every other.
func (c *Client) withTimeout(timeout time.Duration) *Client {
	return &Client{
		base:  c.base,
		token: c.token,
		http:  &http.Client{Timeout: timeout},
	}
}
