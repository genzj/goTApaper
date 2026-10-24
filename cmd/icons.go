package cmd

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/data"
)

// mustReadIcon loads a tray icon from the embedded assets.
//
// A missing icon is fatal rather than tolerated: it means the generated
// data/example_vfsdata.go was not rebuilt, and every other embedded asset --
// the example configuration, the watermark fonts -- is missing with it.
func mustReadIcon(name string) []byte {
	file, err := data.ExampleAssets.Open("/assets/" + name)
	if err != nil {
		logrus.WithField("asset", name).WithError(err).Error(
			"cannot load app resource; run \"make generate\"",
		)
		os.Exit(2)
	}
	defer file.Close()

	icon, err := io.ReadAll(file)
	if err != nil {
		logrus.WithField("asset", name).WithError(err).Error("cannot read app resource")
		os.Exit(2)
	}
	return icon
}
