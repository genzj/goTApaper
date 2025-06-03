package install

import (
	"github.com/ProtonMail/go-autostart"
	"github.com/kardianos/osext"
	"github.com/sirupsen/logrus"
)

const defaultProcName = "goTApaper"

func newApp() (*autostart.App, error) {
	exec, err := osext.Executable()
	if err != nil {
		logrus.WithField("error", err).Errorln("cannot locate the binary file of " + defaultProcName)
		return nil, err
	}
	return &autostart.App{
		Name:        getAppName(),
		DisplayName: "Download latest pictures from many providers and use them as wallpaper.",
		Exec:        []string{exec, "daemon"},
	}, nil
}

func IsInstalled() bool {
	app, err := newApp()
	return err == nil && app != nil && app.IsEnabled()
}

func EnableStartUp(startNow bool) error {
	logrus.Debugf("installing startup files")
	if err := preServiceFileInstall(); err != nil {
		logrus.WithField("error", err).Errorln("failed to execute pre install hook")
		return err
	}

	app, err := newApp()
	if err != nil || app == nil {
		logrus.WithField("error", err).Errorln("cannot create app instance")
		return err
	}

	err = app.Enable()
	if err != nil {
		logrus.WithField("error", err).Errorln("cannot install auto startup files")
		return err
	}

	if err = postServiceFileInstall(startNow); err != nil {
		logrus.WithField("error", err).Errorln("failed to execute post install hook")
		return err
	}
	return err
}

func DisableStartUp() error {
	logrus.Debugf("uninstalling startup files")
	if err := preServiceFileUninstall(); err != nil {
		logrus.WithField("error", err).Warnln("errors in pre uninstall hook")
	}

	app, err := newApp()
	if err != nil || app == nil {
		logrus.WithField("error", err).Errorln("cannot create app instance")
		return err
	}

	err = app.Disable()
	if err != nil {
		logrus.WithField("error", err).Errorln("cannot uninstall auto startup files")
		return err
	}

	if err = postServiceFileUninstall(); err != nil {
		logrus.WithField("error", err).Errorln("failed to execute post uninstall hook")
		return err
	}
	return err
}
