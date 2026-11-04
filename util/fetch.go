package util

import (
	"bytes"
	"fmt"
	"image"
	"net/url"
)

// FetchPicture reads a recorded picture again: from the network, or for the
// fixed channel's file:// URLs, from disk. It is what re-applying a history
// entry and previewing one both use, so they agree on what a URL means.
func FetchPicture(rawURL string) (*bytes.Reader, image.Image, string, error) {
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Scheme == "file" {
		raw, img, format, err := DecodeFromFile(parsed.Path)
		if err != nil {
			return nil, nil, "", fmt.Errorf("cannot read %s again: %w", parsed.Path, err)
		}
		return raw, img, format, nil
	}

	resp, err := GetInType(rawURL, "image/")
	if err != nil {
		return nil, nil, "", fmt.Errorf(
			"cannot download %s again (the provider may have expired the link): %w",
			rawURL, err,
		)
	}
	return DecodeFromResponse(resp)
}
