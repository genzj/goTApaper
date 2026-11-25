package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/service"
)

// watermarkStudio is the running daemon's watermark editor backend, shared by
// the API and the desktop preview so both see the same render.
var watermarkStudio *service.Watermarks

// activeDownloader is the running daemon's downloader, kept so shutting down
// can take a desktop preview off again.
var activeDownloader *service.Downloader

// currentSetter resolves the configured wallpaper setter.
func currentSetter() (setter.Setter, error) {
	name := config.GetString(config.KeyWallpaperSetter)
	v, ok := setter.Setters.Get(name)
	if !ok {
		return nil, fmt.Errorf("setter %q is not registered", name)
	}
	return v.(setter.Setter), nil
}

// showWatermarkPreview sets the editor's rendered preview file as the desktop
// wallpaper.
//
// It hands the setter the preview file itself, which lives in its own
// directory. The wallpaper file is never written, nothing is recorded in the
// history, and on macOS the setter's alternating slot copies are made in the
// preview directory, leaving the real wallpaper's slots alone.
func showWatermarkPreview() (int64, error) {
	if watermarkStudio == nil {
		return 0, errors.New("the watermark editor is not running")
	}
	path, revision, err := watermarkStudio.PreviewFile()
	if err != nil {
		return 0, err
	}
	s, err := currentSetter()
	if err != nil {
		return 0, err
	}
	logrus.WithFields(logrus.Fields{"file": path, "revision": revision}).Info("previewing watermarks on the desktop")
	if err := s.Set(path); err != nil {
		return 0, err
	}
	return revision, nil
}

// revertWatermarkPreview sets the real wallpaper -- the newest history entry's
// file -- on the desktop again.
func revertWatermarkPreview() error {
	entries, err := currentStore().WallpaperHistory(1, 0)
	if err != nil {
		return err
	}
	if len(entries) == 0 || entries[0].FilePath == "" {
		return errors.New("no wallpaper has been applied to go back to")
	}
	path := entries[0].FilePath
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("the wallpaper file %s is gone; refresh to set a new one", path)
	}
	s, err := currentSetter()
	if err != nil {
		return err
	}
	logrus.WithField("file", path).Info("putting the real wallpaper back after a watermark preview")
	return s.Set(path)
}

// revertPreviewOnExit takes a desktop preview off when the application stops,
// so a temporary picture never outlives the process that could revert it.
func revertPreviewOnExit() {
	if activeDownloader == nil {
		return
	}
	if err := activeDownloader.RevertPreviewNow(); err != nil {
		logrus.WithError(err).Warn("cannot take the watermark preview off the desktop")
	}
}
