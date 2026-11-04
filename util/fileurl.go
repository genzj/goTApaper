package util

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// FileURLPath turns a file:// URL into a local path.
//
// A URL's path always starts with a slash, which is right on Unix and wrong on
// Windows: file:///C:/Users/x.jpg has the path "/C:/Users/x.jpg", which
// Windows cannot open. The slash before a drive letter is dropped there, and
// separators are converted.
func FileURLPath(u *url.URL) string {
	p := u.Path
	if runtime.GOOS == "windows" {
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
		// file://server/share/x.jpg names a UNC path.
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			p = "//" + u.Host + p
		}
	}
	return filepath.FromSlash(p)
}

// PathFileURL turns a local path into a file:// URL, the inverse of
// FileURLPath.
func PathFileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		// A Windows drive path: file:///C:/...
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
