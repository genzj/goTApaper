package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/api"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/frontend"
	"github.com/genzj/goTApaper/gui"
	"github.com/genzj/goTApaper/install"
	"github.com/genzj/goTApaper/logging"
	"github.com/genzj/goTApaper/service"
)

var (
	apiEnabled bool
	headless   bool
)

// errAPIServerUnavailable is reported when the interface is asked to open but
// the API server it would show did not start.
var errAPIServerUnavailable = errors.New(
	"the configuration API is not running, so the interface cannot be opened",
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Refresh the desktop periodically, with a tray icon and interface",
	Long: `Refresh the desktop periodically.

Runs a tray icon and a window showing the configuration, history and log
interface. Use --headless on a machine with no desktop session to run the
refresh loop and the localhost API alone.`,
	Run: func(cmd *cobra.Command, args []string) {
		daemon()
	},
}

// guiCmd is an alias, for discoverability: someone looking for the interface
// is more likely to try "gui" than "daemon".
var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Open the tray icon and interface (an alias for \"daemon\")",
	Run: func(cmd *cobra.Command, args []string) {
		daemon()
	},
}

const (
	stagePreRefresh = iota
	stagePostRefresh
)

type cycleUpdateCallback func(int, *channel.PictureMeta, error)
type nextCycleChannel = chan []string
type nextCycleWaitChannel = <-chan []string
type nextCycleTrigger func(channels []string)

func init() {
	daemonCmd.PersistentFlags().BoolVar(
		&apiEnabled, "api", true,
		"serve the configuration API on 127.0.0.1",
	)
	daemonCmd.PersistentFlags().BoolVar(
		&headless, "headless", false,
		"run without the tray icon and window, for a machine with no desktop session",
	)
	daemonCmd.PersistentFlags().Uint32P(
		"interval", "i", config.DefaultDaemonInterval, "interval between two refreshes",
	)
	RootCmd.AddCommand(daemonCmd)
	RootCmd.AddCommand(guiCmd)
}

func initDaemon(nextCycleCh nextCycleWaitChannel, callback cycleUpdateCallback) {
	go func() {
		var channels []string = nil
		for {
			interval := config.GetInt(config.KeyDaemonInterval)
			logrus.WithField("interval", interval).Debug("refresh over, going to sleep")
			select {
			case <-time.After(time.Duration(interval) * time.Second):
				logrus.WithField("interval", interval).Debug("awake from sleep")
				channels = nil
			case channels = <-nextCycleCh:
				logrus.Debug("trigger next cycle before interval timeout")
			}
			callback(stagePreRefresh, nil, nil)
			meta, err := refresh(channels)
			callback(stagePostRefresh, meta, err)
		}
	}()
}

// apiServer is the running API server, if one was started.
var apiServer *api.Server

// startAPIServer brings up the localhost API. A failure is logged rather than
// fatal: the daemon's job is to set wallpapers, and it should keep doing that
// even if the configuration interface could not be opened.
func startAPIServer() {
	if !apiEnabled {
		logrus.Debug("API server disabled")
		return
	}

	// Installed before Start, so the /ui/ routes have something to serve. A
	// build without the bundle reports the interface as unavailable rather
	// than failing to start.
	api.SetUIAssets(frontend.MustAssets())

	srv, err := api.Start(api.Options{
		AppDir:    config.AppDir(),
		Port:      config.GetInt(config.KeyAPIPort),
		Config:    service.NewConfig(currentStore()),
		Wallpaper: service.NewWallpaper(currentStore(), refresh, reapply),
		Logs:      service.NewLogs(),
	})
	if err != nil {
		logrus.WithError(err).Error("cannot start the API server, continuing without it")
		return
	}
	apiServer = srv
}

// publishConfigChange forwards a configuration change onto the event stream,
// so an open interface reloads instead of polling.
//
// The daemon observes changes already, to react to them immediately; this
// hangs off the same observer rather than adding a second one, which keeps the
// UI from ever seeing a change the daemon has not also acted on.
func publishConfigChange(key string) {
	if apiServer == nil {
		return
	}
	apiServer.PublishEvent(api.EventConfigChanged, map[string]string{"key": key})
}

// startRefreshLoop brings up the API server, the configuration observer and
// the periodic refresh, and returns the trigger for an immediate one.
//
// Shared by the windowed and headless paths, so the two cannot drift on what a
// running daemon actually does.
func startRefreshLoop(callback cycleUpdateCallback) nextCycleTrigger {
	startAPIServer()

	nextCycleCh := make(nextCycleChannel)

	nextCycle := func(channels []string) {
		nextCycleCh <- channels
	}

	config.Observe("*", func(key string, old, new interface{}) {
		logrus.WithField("key", key).WithField("old", old).WithField("new", new).Debug("config change triggers refresh")
		publishConfigChange(key)
		nextCycle(nil)
	})

	initDaemon(nextCycleWaitChannel(nextCycleCh), callback)
	// Sweep once at startup, so a cap lowered while the daemon was stopped
	// takes effect without waiting for the next refresh.
	sweepHistory(logrus.WithField("stage", "startup"))
	return nextCycle
}

func onExit() {
	// Flushed first, so the last entries reach the file before the handle
	// goes.
	defer func() {
		if err := logging.Close(); err != nil {
			logrus.WithError(err).Debug("cannot close the log file")
		}
	}()

	if apiServer == nil {
		return
	}
	// Bounded, so a hung request cannot keep the application alive after the
	// user has asked to quit.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		logrus.WithError(err).Debug("API server did not shut down cleanly")
	}
}

func daemon() {
	logrus.Infoln("starting daemon...")

	if headless {
		runHeadless()
		return
	}
	runWindowed()
}

// runWindowed runs the tray and the interface. It blocks on the UI event loop,
// which must own the main goroutine.
func runWindowed() {
	installSignalHandler(nil)

	err := gui.Run(gui.Options{
		Name:        AppName,
		Description: "Desktop wallpaper manager",

		// Rebuilt per navigation, because the ticket inside it is single-use.
		SessionURL: func() (string, error) {
			if apiServer == nil {
				return "", errAPIServerUnavailable
			}
			return apiServer.SessionURL()
		},

		Icon:         mustReadIcon("icons8-sheet-of-paper-systray.ico"),
		TemplateIcon: mustReadIcon("icons8-sheet-of-paper-template.ico"),

		IsInstalled:  install.IsInstalled,
		EnableLogin:  func() error { return install.EnableStartUp(false) },
		DisableLogin: install.DisableStartUp,

		OnReady: func(tray *gui.Tray) {
			// The refresh loop is started from here rather than before
			// gui.Run, so the first callback cannot reach a tray that does not
			// exist yet.
			nextCycle := startRefreshLoop(trayCallback(tray))
			trayRefresh = nextCycle
			nextCycle(nil)
		},
		Refresh: func(channels []string) {
			if trayRefresh != nil {
				trayRefresh(channels)
			}
		},
		OnExit: onExit,
	})
	if err != nil {
		logrus.WithError(err).Fatal("the desktop interface could not start; " +
			"use \"goTApaper daemon --headless\" to run without it")
	}
}

// trayRefresh triggers a refresh from the tray. Package-level because the tray
// menu is built before the refresh loop it drives exists.
var trayRefresh nextCycleTrigger

// trayCallback adapts the refresh loop's progress reports to the tray.
func trayCallback(tray *gui.Tray) cycleUpdateCallback {
	return func(stage int, meta *channel.PictureMeta, err error) {
		tray.ShowPicture(meta, err)
		tray.Refreshing(stage == stagePreRefresh)

		// Also announced on the event stream, so an open interface shows
		// progress for a refresh the tray started.
		if apiServer == nil {
			return
		}
		if stage == stagePreRefresh {
			apiServer.PublishEvent(api.EventRefreshStarted, nil)
			return
		}
		apiServer.PublishEvent(api.EventRefreshFinished, refreshEventData(meta, err))
	}
}

func refreshEventData(meta *channel.PictureMeta, err error) any {
	data := map[string]string{}
	if meta != nil {
		data["channel"] = meta.ChannelKey
		data["type"] = meta.Channel
		data["title"] = meta.Title
		data["credit"] = meta.Credit
	}
	if err != nil {
		data["error"] = err.Error()
	}
	return data
}

// runHeadless runs the refresh loop with no tray and no window.
//
// This is the path for a machine with no desktop session at all -- a Linux
// server or a container with no GTK and no webview -- and the fallback when
// the interface cannot start. It is also what makes the API usable over SSH.
func runHeadless() {
	logrus.Info("running without the desktop interface")

	stop := make(chan struct{})
	installSignalHandler(stop)

	nextCycle := startRefreshLoop(func(int, *channel.PictureMeta, error) {})
	nextCycle(nil)

	<-stop
}

// installSignalHandler runs the shutdown path on SIGINT and SIGTERM.
//
// Without it a `kill` would leave the endpoint file behind advertising a port
// nothing is listening on, and the last log lines unflushed. stop is closed to
// release the headless path; it is nil in the windowed case, where the UI
// event loop ends the process instead.
func installSignalHandler(stop chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-signals
		logrus.WithField("signal", sig.String()).Info("shutting down")
		onExit()

		if stop != nil {
			close(stop)
			return
		}
		// The UI owns the process in the windowed case, and there is no
		// portable way to ask it to quit from here, so exit directly now that
		// the server and the log are closed.
		os.Exit(0)
	}()
}
