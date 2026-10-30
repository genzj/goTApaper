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
		// The interface, not "daemon": a login item exists because someone
		// ticked "Start at login" in the tray, and wants the tray back.
		Exec: []string{exec, "gui"},
	}, nil
}

// MigrateStartUp rewrites an existing login item so it starts what this
// version registers.
//
// Login items written before the interface existed start "daemon", which is
// now the headless daemon: left alone, the next login would bring back no
// tray. Only the login item file is rewritten -- the platform hooks are
// skipped on purpose, because on macOS the install hook boots the agent out of
// launchd, which would stop this very process if launchd started it.
func MigrateStartUp() {
	if !IsInstalled() {
		return
	}
	app, err := newApp()
	if err != nil || app == nil {
		return
	}
	if err := app.Enable(); err != nil {
		logrus.WithError(err).Debug("cannot update the login item")
	}
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
