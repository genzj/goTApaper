package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/genzj/goTApaper/api"
	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/service"
	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var apiEnabled bool

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Refresh and set desktop periodically",
	Long:  `Refresh and set desktop periodically`,
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
	daemonCmd.PersistentFlags().Uint32P(
		"interval", "i", config.DefaultDaemonInterval, "interval between two refreshes",
	)
	RootCmd.AddCommand(daemonCmd)
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

	srv, err := api.Start(api.Options{
		AppDir:    config.AppDir(),
		Port:      config.GetInt(config.KeyAPIPort),
		Config:    service.NewConfig(currentStore()),
		Wallpaper: service.NewWallpaper(currentStore(), refresh),
	})
	if err != nil {
		logrus.WithError(err).Error("cannot start the API server, continuing without it")
		return
	}
	apiServer = srv
}

func onReady() {
	startAPIServer()

	nextCycleCh := make(nextCycleChannel)

	nextCycle := func(channels []string) {
		nextCycleCh <- channels
	}

	config.Observe("*", func(key string, old, new interface{}) {
		logrus.WithField("key", key).WithField("old", old).WithField("new", new).Debug("config change triggers refresh")
		nextCycle(nil)
	})

	callback := initSystray(nextCycle)
	initDaemon(nextCycleWaitChannel(nextCycleCh), callback)
	nextCycle(nil)
}

func onExit() {
	if apiServer == nil {
		return
	}
	// Bounded, so a hung request cannot keep the tray icon alive after the
	// user has asked to quit.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		logrus.WithError(err).Debug("API server did not shut down cleanly")
	}
}

func daemon() {
	logrus.Infoln("starting daemon...")
	installSignalHandler()
	systray.Run(onReady, onExit)
}

// installSignalHandler runs the shutdown path on SIGINT and SIGTERM.
//
// The tray's own exit callback only fires when the user quits from the menu,
// so without this a `kill` would leave the endpoint file behind advertising a
// port nothing is listening on.
func installSignalHandler() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-signals
		logrus.WithField("signal", sig.String()).Info("shutting down")
		onExit()
		systray.Quit()
	}()
}
