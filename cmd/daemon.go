package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/genzj/goTApaper/api"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/frontend"
	"github.com/genzj/goTApaper/install"
	"github.com/genzj/goTApaper/logging"
	"github.com/genzj/goTApaper/service"
)

var apiEnabled bool

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Refresh the desktop periodically, with no tray icon or window",
	Long: `Refresh the desktop periodically, with no tray icon or window.

This is the headless daemon, for a machine with no desktop session -- a
server, a container, a session reached over SSH -- or for running under a
service manager. It serves the localhost API like the desktop interface does,
so the CLI and a browser can still drive it.

For the tray icon and window, use "goTApaper gui". Unlike the interface, the
daemon is not limited to one instance: running two against one application
directory gives its database two writers, which is left to you to avoid.`,
	Run: func(cmd *cobra.Command, args []string) {
		runHeadless()
	},
}

func init() {
	addDaemonFlags(daemonCmd.PersistentFlags())
	RootCmd.AddCommand(daemonCmd)
}

// addDaemonFlags registers the flags the headless daemon and the interface
// share, since both run the same refresh loop.
func addDaemonFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&apiEnabled, "api", true, "serve the configuration API on 127.0.0.1")
	flags.Uint32P("interval", "i", config.DefaultDaemonInterval, "interval between two refreshes")
}

// runSchedule refreshes every daemon.interval seconds, or sooner when woken.
//
// It only ever schedules: the downloader decides whether that starts a
// download now or after the one in flight, so the timer, a configuration
// change, the tray and the API can never run two downloads at once.
func runSchedule(wake <-chan struct{}, d *service.Downloader) {
	go func() {
		for {
			interval := config.GetInt(config.KeyDaemonInterval)
			logrus.WithField("interval", interval).Debug("going to sleep until the next refresh")
			select {
			case <-time.After(time.Duration(interval) * time.Second):
				logrus.WithField("interval", interval).Debug("awake from sleep")
			case <-wake:
				logrus.Debug("woken before the interval elapsed")
			}
			d.Schedule()
		}
	}()
}

// apiServer is the running API server, if one was started.
var apiServer *api.Server

// startAPIServer brings up the localhost API. A failure is logged rather than
// fatal: the daemon's job is to set wallpapers, and it should keep doing that
// even if the configuration interface could not be opened.
func startAPIServer(d *service.Downloader, lc api.Lifecycle) {
	if !apiEnabled {
		logrus.Debug("API server disabled")
		return
	}

	// Installed before Start, so the /ui/ routes have something to serve. A
	// build without the bundle reports the interface as unavailable rather
	// than failing to start.
	api.SetUIAssets(frontend.MustAssets())

	srv, err := api.Start(api.Options{
		AppDir:     config.AppDir(),
		Port:       config.GetInt(config.KeyAPIPort),
		Config:     service.NewConfig(currentStore()),
		Wallpaper:  service.NewWallpaper(currentStore()),
		Downloader: d,
		Logs:       service.NewLogs(),
		Version:    Version,
		Lifecycle:  lc,
		RequireToken: func() bool {
			return config.GetBool(config.KeyAPIRequireToken)
		},
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

// startRefreshLoop brings up the downloader, the API server, the
// configuration observer and the periodic refresh, and starts the first
// refresh. onChange, if set, hears every download start and finish.
//
// Shared by the windowed and headless paths, so the two cannot drift on what a
// running daemon actually does.
func startRefreshLoop(lc api.Lifecycle, onChange func(service.DownloadEvent)) *service.Downloader {
	// A login item written by an older version starts "daemon", which is now
	// the headless one. Rewriting it here means the next login gets the
	// interface back, whichever command this start happened to be.
	install.MigrateStartUp()

	d := service.NewDownloader(currentStore(), refresh, reapply)
	if onChange != nil {
		d.OnChange(onChange)
	}

	startAPIServer(d, lc)

	// Buffered and sent to without blocking: several changes in quick
	// succession want one refresh, and an observer must never stall waiting
	// for the schedule to come round, which it used to do for as long as a
	// download took.
	wake := make(chan struct{}, 1)
	config.Observe("*", func(key string, old, new interface{}) {
		logrus.WithField("key", key).WithField("old", old).WithField("new", new).Debug("config change triggers refresh")
		publishConfigChange(key)
		select {
		case wake <- struct{}{}:
		default:
		}
	})

	runSchedule(wake, d)
	// Sweep once at startup, so a cap lowered while the daemon was stopped
	// takes effect without waiting for the next refresh.
	sweepHistory(logrus.WithField("stage", "startup"))
	d.Schedule()
	return d
}

var exitOnce sync.Once

// onExit releases what a running daemon holds: the API server, and with it
// the endpoint file, and the log file. Safe to call more than once, since a
// signal and the UI's own shutdown can both reach it.
func onExit() {
	exitOnce.Do(func() {
		// Deferred so the log file closes last and still records the API
		// server going away.
		defer func() {
			if err := logging.Close(); err != nil {
				logrus.WithError(err).Debug("cannot close the log file")
			}
		}()

		if apiServer == nil {
			return
		}
		// Bounded, so a hung request cannot keep the application alive after
		// the user has asked to quit.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := apiServer.Shutdown(ctx); err != nil {
			logrus.WithError(err).Debug("API server did not shut down cleanly")
		}
	})
}

// runHeadless runs the refresh loop with no tray and no window, until a
// signal or an API request stops it.
func runHeadless() {
	logrus.Info("starting the headless daemon")

	stop := make(chan struct{})
	lc := newLifecycle(func() { close(stop) })
	installSignalHandler(lc)

	startRefreshLoop(lc, nil)
	<-stop

	onExit()
	lc.restartIfRequested()
}

// installSignalHandler stops the process on SIGINT and SIGTERM, through the
// same path an API request takes.
//
// Without it a `kill` would leave the endpoint file behind advertising a port
// nothing is listening on, and the last log lines unflushed.
func installSignalHandler(lc *lifecycle) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-signals
		logrus.WithField("signal", sig.String()).Info("shutting down")
		lc.Stop()
	}()
}
