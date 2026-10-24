package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// RotateOptions configures the rotating log file.
type RotateOptions struct {
	// Path is the active log file. An empty path disables file logging.
	Path string
	// MaxSizeMB is the size at which the active file is rotated.
	MaxSizeMB int
	// MaxBackups is how many rotated files to keep. Together with MaxSizeMB
	// this is the entire disk budget: MaxSizeMB * (MaxBackups + 1).
	MaxBackups int
}

// Defaults for the rotating file, mirrored by the setting defaults in
// config/settings_schema.go.
const (
	DefaultMaxSizeMB  = 10
	DefaultMaxBackups = 5
)

// NewRotatingWriter opens the rotating log file described by opts, creating
// its directory if needed. It returns nil when opts.Path is empty, which the
// hook treats as "no file logging".
//
// lumberjack.MaxAge is deliberately left unset. Size and backup count already
// bound the disk budget, and adding an age limit would mean two independent
// rules deciding when a log line disappears.
func NewRotatingWriter(opts RotateOptions) (io.WriteCloser, error) {
	if opts.Path == "" {
		return nil, nil
	}

	if opts.MaxSizeMB < 1 {
		opts.MaxSizeMB = DefaultMaxSizeMB
	}
	if opts.MaxBackups < 0 {
		opts.MaxBackups = DefaultMaxBackups
	}

	if dir := filepath.Dir(opts.Path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create the log directory %s: %w", dir, err)
		}
	}

	return &lumberjack.Logger{
		Filename:   opts.Path,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		LocalTime:  true,
		Compress:   false,
	}, nil
}

// FileInfo describes one log file on disk, so a client can offer it for
// download without being told where logs live.
type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	// ModTime is the Unix timestamp of the last write.
	ModTime int64 `json:"modTime"`
	// Active marks the file currently being written to.
	Active bool `json:"active"`
}

// Files lists the active log file and its rotated siblings, newest first.
//
// lumberjack names a backup by inserting a timestamp before the extension --
// "gotapaper-2026-10-23T09-00-00.000.log" next to "gotapaper.log" -- so the
// siblings are found by matching that prefix and extension rather than by
// asking lumberjack, which exposes no listing.
func Files(activePath string) ([]FileInfo, error) {
	if activePath == "" {
		return nil, nil
	}

	dir := filepath.Dir(activePath)
	base := filepath.Base(activePath)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext)

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing has been logged to disk yet. An empty list is the honest
			// answer, not an error.
			return nil, nil
		}
		return nil, err
	}

	var out []FileInfo
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}
		name := dirEntry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil {
			continue
		}
		out = append(out, FileInfo{
			Name:    name,
			Path:    filepath.Join(dir, name),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Active:  name == base,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		// The active file first, then rotated files newest to oldest, which is
		// the order someone investigating a problem reads them in.
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		return out[i].ModTime > out[j].ModTime
	})
	return out, nil
}
