package cmd

import (
	"time"

	"github.com/Sirupsen/logrus"
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
	ch := make(chan error)
	for {
		go func() {
			defer func() { ch <- nil }()
			refresh()
		}()
		<-ch
		interval := viper.GetInt("daemon.interval")
		logrus.WithField("interval", interval).Debug("refresh over, sleep")
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
