// +build linux

package actor

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// Xfce4Setter works for XFCE4
type Xfce4Setter int

// Set wallpaper with xfconf-query
func (Xfce4Setter) Set(filename string) error {
	path, err := filepath.Abs(filename)
	if err != nil {
		return err
	}

	cmd := exec.Command(
		"xfconf-query",
		"-c",
		"xfce4-desktop",
		"-l",
	)
	bs, err := cmd.Output()
	if err != nil {
		logrus.Errorf("cannot read xfce4 desktop settings: %s", string(bs))
	}

	for _, line := range strings.Split(string(bs), "\n") {
		logrus.Debugf("xfce-config output %v", line)
		if strings.HasSuffix(line, "last-image") {
			if err = setWithCommand(
				"xfconf-query",
				"-c",
				"xfce4-desktop",
				"-p",
				line,
				"-s",
				path,
			); err != nil {
				logrus.Errorf(
					"error during setting %s to %s: %s", line, path, err,
				)
				return err
			}
		} else if strings.HasSuffix(line, "image-style") {
			if err = setWithCommand(
				"xfconf-query",
				"-c",
				"xfce4-desktop",
				"-p",
				line,
				"-s",
				"5",
			); err != nil {
				logrus.Errorf("error during setting %s to 5: %s", line, err)
				return err
			}
		}
	}

	return nil
}

func init() {
	Setters.Register("xfce4", Xfce4Setter(0))
}
