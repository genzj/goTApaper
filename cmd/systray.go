package cmd

import (
	"io/ioutil"
	"os"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/data"
	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"
)

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

func initSystray(nextCycle nextCycleTrigger) cycleUpdateCallback {
	systrayIcon := mustReadIcon("icons8-sheet-of-paper-systray.ico")
	templateIcon := mustReadIcon("icons8-sheet-of-paper-template.ico")

	systray.SetTemplateIcon(templateIcon, systrayIcon)
	systray.SetTooltip("goTApaper")

	mError := systray.AddMenuItem("error", "")
	mError.Disable()
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
				nextCycle(nil)
			}
		}
	}()

	return func(stage int, meta *channel.PictureMeta, err error) {
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
			mTitle.SetTitle("")
			mTitle.Hide()
			mChannel.SetTitle("")
			mChannel.Hide()
			mCredit.SetTitle("")
			mCredit.Hide()
			mUpdateTime.SetTitle("")
			mUpdateTime.Hide()
		}
		if err != nil {
			mError.SetTitle(err.Error())
			mError.Show()
		} else {
			mError.SetTitle("")
			mError.Hide()
		}
		switch stage {
		case stagePreRefresh:
			mRefresh.SetTitle("Refreshing...")
			mRefresh.Disable()
		case stagePostRefresh:
			mRefresh.SetTitle("Refresh")
			mRefresh.Enable()
		}
	}
}
