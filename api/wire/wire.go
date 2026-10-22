// Package wire holds the few things the API server and its clients both need:
// where the handshake files live, and the shape of an error response.
//
// It exists so a client does not have to import the server package, and with
// it every request handler, just to learn a filename.
package wire

import "path/filepath"

const (
	// TokenFileName holds the bearer token, readable only by its owner.
	TokenFileName = "api-token"

	// EndpointFileName records the listening address, so a client can find a
	// running daemon without being told the port.
	EndpointFileName = "api-endpoint"

	// BasePath prefixes every route.
	BasePath = "/api/v1"
)

// TokenPath returns the token file's location within an application directory.
func TokenPath(appDir string) string { return filepath.Join(appDir, TokenFileName) }

// EndpointPath returns the endpoint file's location.
func EndpointPath(appDir string) string { return filepath.Join(appDir, EndpointFileName) }

// ErrorBody is the shape every failing route returns. Clients decode it to
// recover the server's own message rather than inventing their own.
type ErrorBody struct {
	Error string `json:"error"`
	// UsedBy names the channels blocking a template delete, so a client can
	// offer to inline it instead of just reporting a conflict.
	UsedBy []string `json:"usedBy,omitempty"`
	// Expected lists the acceptable values when a value was rejected.
	Expected []string `json:"expected,omitempty"`
}
