package cmd

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/data"
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

type cycleCompleteCallback func(*channel.PictureMeta, error)
type nextCycleChannel = chan cycleCompleteCallback
type nextCycleWaitChannel = <-chan cycleCompleteCallback
type nextCycleTrigger func(done cycleCompleteCallback)

func init() {
	daemonCmd.PersistentFlags().Uint32P("interval", "i", config.DefaultDaemonInterval, "interval between two refreshes")
	viper.BindPFlag("daemon.interval", daemonCmd.PersistentFlags().Lookup("interval"))
	RootCmd.AddCommand(daemonCmd)
}

func mustReadIcon(name string) []byte {
	file, err := data.ExampleAssets.Open("/assets/" + name)
	if err != nil {
		logrus.WithError(err).Errorln("cannot load app resource")
		os.Exit(2)
	}
	defer file.Close()
	icon, err := ioutil.ReadAll(file)
	if err != nil {
		logrus.WithError(err).Errorln("cannot read app resource")
		os.Exit(2)
	}
	return icon
}

func initSystray(nextCycle nextCycleTrigger) {
	systrayIcon := mustReadIcon("icons8-sheet-of-paper-systray.ico")
	templateIcon := mustReadIcon("icons8-sheet-of-paper-template.ico")

	systray.SetTemplateIcon(templateIcon, systrayIcon)
	systray.SetTooltip("goTApaper")

	mTitle := systray.AddMenuItem("title", "")
	mTitle.Disable()
	mCredit := systray.AddMenuItem("credit", "")
	mCredit.Disable()
	mChannel := systray.AddMenuItem("channel", "")
	mChannel.Disable()
	mUpdateTime := systray.AddMenuItem("update-time", "")
	mUpdateTime.Disable()

	mTitle.Hide()
	mCredit.Hide()
	mChannel.Hide()
	mUpdateTime.Hide()

	systray.AddSeparator()

	mRefresh := systray.AddMenuItem("Refresh", "Refresh desktop now")
	mQuitOrig := systray.AddMenuItem("Quit", "Quit the whole app")

	go func() {
		for {
			select {
			case <-mQuitOrig.ClickedCh:
				logrus.Debugln("Requesting quit")
				systray.Quit()
			case <-mRefresh.ClickedCh:
				logrus.Debugln("Requesting refresh")
				mRefresh.SetTitle("Refreshing...")
				mRefresh.Disable()
				nextCycle(func(meta *channel.PictureMeta, err error) {
					if meta != nil {
						mTitle.SetTitle(meta.Title)
						mTitle.Show()
						mCredit.SetTitle(meta.Credit)
						mCredit.Show()
						mChannel.SetTitle(meta.Channel)
						mChannel.Show()
						mUpdateTime.SetTitle(meta.UploadTime.Local().String())
						mUpdateTime.Show()
					} else {
						mTitle.Hide()
						mChannel.Hide()
						mCredit.Hide()
						mUpdateTime.Hide()
					}
					mRefresh.SetTitle("Refresh")
					mRefresh.Enable()
				})
			}
		}
	}()

	// initiate a refreshing at startup
	mRefresh.ClickedCh <- struct{}{}
}

func initDaemon(nextCycleCh nextCycleWaitChannel) {
	go func() {
		for {
			interval := viper.GetInt("daemon.interval")
			logrus.WithField("interval", interval).Debug("refresh over, going to sleep")
			select {
			case <-time.After(time.Duration(interval) * time.Second):
				logrus.WithField("interval", interval).Debug("awake from sleep")
				refresh(nil)
			case done := <-nextCycleCh:
				logrus.Debug("trigger next cycle before interval timeout")
				meta, err := refresh(nil)
				done(meta, err)
			}
		}
	}()
}

func onReady() {
	nop := func(pm *channel.PictureMeta, e error) {}
	//api.StartApiServer()
	nextCycleCh := make(nextCycleChannel)

	nextCycle := func(done cycleCompleteCallback) {
		nextCycleCh <- done
	}

	config.Observe("*", func(key string, old, new interface{}) {
		logrus.WithField("key", key).WithField("old", old).WithField("new", new).Debug("config change triggers refresh")
		nextCycle(nop)
	})

	initSystray(nextCycle)
	initDaemon(nextCycleWaitChannel(nextCycleCh))
}

func onExit() {
	// clean up here
}

func daemon() {
	logrus.Infoln("starting daemon...")
	systray.Run(onReady, onExit)
}
