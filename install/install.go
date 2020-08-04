package install

import (
	"github.com/ProtonMail/go-autostart"
	"github.com/kardianos/osext"
	"github.com/sirupsen/logrus"
)

const defaultProcName = "goTApaper"

func InstallStartUp() error {
	logrus.Debugf("installing startup files")
	if err := preServiceFileInstall(); err != nil {
		logrus.WithField("error", err).Errorln("failed to execute pre install hook")
		return err
	}

	exec, err := osext.Executable()
	if err != nil {
		logrus.WithField("error", err).Errorln("cannot locate the binary file of " + defaultProcName)
		return err
	}

	app := &autostart.App{
		Name:        getAppName(),
		DisplayName: "Download latest pictures from many providers and use them as wallpaper.",
		Exec:        []string{exec, "daemon"},
	}
	err = app.Enable()
	if err != nil {
		logrus.WithField("error", err).Errorln("cannot install auto startup files")
		return err
	}

	if err = postServiceFileInstall(); err != nil {
		logrus.WithField("error", err).Errorln("failed to execute post install hook")
		return err
	}
	return err
}
