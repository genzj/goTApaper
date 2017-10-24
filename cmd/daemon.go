package cmd

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/api"
	"github.com/genzj/goTApaper/config"
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

func init() {
	daemonCmd.PersistentFlags().Uint32P("interval", "i", config.DefaultDaemonInterval, "interval between two refreshes")
	viper.BindPFlag("daemon.interval", daemonCmd.PersistentFlags().Lookup("interval"))
	RootCmd.AddCommand(daemonCmd)
}

func daemon() {
	api.StartApiServer()
	ch := make(chan int)

	config.Observe("*", func(key string, old, new interface{}) {
		logrus.WithField("key", key).WithField("old", old).WithField("new", new).Debug("config change triggers refresh")
		ch <- 1
	})

	for {
		refresh()
		interval := viper.GetInt("daemon.interval")
		logrus.WithField("interval", interval).Debug("refresh over, going to sleep")
		select {
		case <-time.After(time.Duration(interval) * time.Second):
			logrus.WithField("interval", interval).Debug("awake from sleep")
		case <-ch:
			logrus.Debug("triggered by config observer")
		}
	}
}
