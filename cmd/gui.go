package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/api/client"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/gui"
	"github.com/genzj/goTApaper/install"
	"github.com/genzj/goTApaper/service"
)

// errAPIServerUnavailable is reported when the interface is asked to open but
// the API server it would show did not start.
var errAPIServerUnavailable = errors.New(
	"the configuration API is not running, so the interface cannot be opened",
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Run the tray icon and window, refreshing the desktop periodically",
	Long: `Run the tray icon and window, refreshing the desktop periodically.

This is the desktop application: everything the headless "daemon" does, plus a
tray menu and a window for the configuration, the history and the logs.

Only one runs per application directory. Launching it again shows the window
of the one already running. If a headless daemon is serving the same
directory, the interface asks it to stop and takes over, so the database never
has two writers.`,
	Run: func(cmd *cobra.Command, args []string) {
		runWindowed()
	},
}

func init() {
	addDaemonFlags(guiCmd.PersistentFlags())
	RootCmd.AddCommand(guiCmd)
}

// runWindowed runs the tray and the interface. It blocks on the UI event loop,
// which must own the main goroutine.
func runWindowed() {
	logrus.Info("starting the desktop interface")

	// Set by OnReady, before which nothing can ask to stop.
	var tray *gui.Tray
	lc := newLifecycle(func() {
		if tray != nil {
			tray.Quit()
		}
	})
	installSignalHandler(lc)

	err := gui.Run(gui.Options{
		Name:        AppName,
		Description: "Desktop wallpaper manager",
		InstanceID:  instanceID(config.AppDir()),

		// Rebuilt every time, because the ticket inside it is single-use.
		SessionURL: func() (string, error) {
			if apiServer == nil {
				return "", errAPIServerUnavailable
			}
			return apiServer.SessionURL()
		},
		UIURL: func() (string, error) {
			if apiServer == nil {
				return "", errAPIServerUnavailable
			}
			return apiServer.UIURL(), nil
		},
		Token: func() string {
			if apiServer == nil {
				return ""
			}
			return apiServer.Token()
		},

		Icon:         mustReadIcon("icons8-sheet-of-paper-systray.ico"),
		TemplateIcon: mustReadIcon("icons8-sheet-of-paper-template.ico"),

		IsInstalled:  install.IsInstalled,
		EnableLogin:  func() error { return install.EnableStartUp(false) },
		DisableLogin: install.DisableStartUp,

		OnReady: func(t *gui.Tray) {
			tray = t
			// By now Wails has made sure this is the only interface for the
			// directory, so a daemon still answering here is a headless one.
			takeOverFromDaemon()
			// The refresh loop is started from here rather than before
			// gui.Run, so the first download cannot report to a tray that does
			// not exist yet.
			trayDownloader = startRefreshLoop(lc, trayListener(t))
		},
		Refresh: func(channels []string) {
			if trayDownloader == nil {
				return
			}
			// Busy is not worth reporting: the tray disables its Refresh item
			// while a download runs, so this only races a download that
			// started a moment ago.
			if err := trayDownloader.Refresh(channels, false, "tray"); err != nil {
				logrus.WithError(err).Debug("tray refresh not started")
			}
		},
		OnExit: onExit,
		// Last, once the single-instance lock is released, so a restarted
		// interface can take it. On macOS nothing runs after this returns.
		AfterExit: lc.restartIfRequested,
	})
	if err != nil {
		logrus.WithError(err).Fatal("the desktop interface could not start; " +
			"use \"goTApaper daemon\" to run without it")
	}
}

// instanceID names the single interface allowed per application directory.
//
// Per directory rather than global, because the limit exists for the
// database's sake: two interfaces on two directories have two databases and
// are fine, which is also what lets a throwaway --app-dir be tried alongside
// the real one. Hashed so the path does not appear in a lock file name.
func instanceID(appDir string) string {
	sum := sha256.Sum256([]byte(appDir))
	return "com.github.genzj.goTApaper." + hex.EncodeToString(sum[:8])
}

// takeOverFromDaemon stops a headless daemon serving this directory, so the
// interface is the database's only writer.
//
// The common case is an upgrade: a login item written by an older version
// starts "daemon", which is now headless, and the user then opens the
// application to get the tray back.
func takeOverFromDaemon() {
	c := client.Discover(config.AppDir())
	if c == nil {
		return
	}

	l := logrus.WithField("daemon", c.Addr())
	l.Info("a daemon is already serving this directory, asking it to stop")
	if err := c.Stop(); err != nil {
		l.WithError(err).Warn("cannot stop the running daemon; both will write the database")
		return
	}
	if !c.Gone(15 * time.Second) {
		l.Warn("the running daemon did not stop; both will write the database")
	}
}

// trayDownloader is what the tray's Refresh item drives. Package-level
// because the tray menu is built before the refresh loop it drives exists.
var trayDownloader *service.Downloader

// trayListener shows download progress in the tray. The event stream hears
// the same downloads from the API server's own subscription.
func trayListener(tray *gui.Tray) func(service.DownloadEvent) {
	return func(e service.DownloadEvent) {
		tray.Refreshing(!e.Finished)
		if e.Finished {
			tray.ShowPicture(e.Meta, e.Err)
		}
	}
}
