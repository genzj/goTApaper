package util

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// FileDigest returns the first 12 hex digits of a file's SHA-256, or "" if it
// cannot be read.
//
// It is for the log, not for security: matching digests across log lines is
// how to tell whether the picture on the desktop is the one just written, or
// one written earlier under the same path.
func FileDigest(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
