package cmd

import (
	"time"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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
	daemonCmd.PersistentFlags().Uint32P("interval", "i", config.DefaultDaemonInterval, "interval between two refreshes")
	viper.BindPFlag("daemon.interval", daemonCmd.PersistentFlags().Lookup("interval"))
	RootCmd.AddCommand(daemonCmd)
}

func initDaemon(nextCycleCh nextCycleWaitChannel, callback cycleUpdateCallback) {
	go func() {
		var channels []string = nil
		for {
			interval := viper.GetInt("daemon.interval")
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

func onReady() {
	//api.StartApiServer()
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
	// clean up here
}

func daemon() {
	logrus.Infoln("starting daemon...")
	systray.Run(onReady, onExit)
}
