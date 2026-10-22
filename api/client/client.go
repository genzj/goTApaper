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
	"os"
	"strings"
	"time"

	"github.com/genzj/goTApaper/api/wire"
)

// requestTimeout bounds a call to the daemon. Generous enough for a refresh to
// be queued, short enough that a wedged daemon does not hang the CLI.
const requestTimeout = 10 * time.Second

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

// TODO: wrap the remaining endpoints (channels, channel templates, watermark
// layers, history, refresh) so every mutating CLI command routes through a
// running daemon rather than only the settings ones. Do returns typed errors
// and decodes arbitrary payloads already, so each is a thin method; the
// commands that still write directly are correct today but do not give the
// daemon its immediate-reaction signal.
