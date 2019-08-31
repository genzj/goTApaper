package channel

import (
	"bytes"
	"errors"
	"image"
	"io/ioutil"

	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	bingChannelName = "bing-wallpaper"
	bingBaseURL     = "https://www.bing.com"
	bingGalleryURL  = bingBaseURL + "/HPImageArchive.aspx?format=js&mbl=1&idx=-1&n=5"
)

var sizeArray = []struct {
	width int
	size  string
}{
	//{width: 1200, size: "1920x1200"},
	{width: 1920, size: "1920x1080"},
}

type bingItem struct {
	Copyright     string
	URL           string
	URLBase       string
	FullStartDate string
}

type bingResponse struct {
	Images []bingItem
}

func bingFindFirstFit(setting *viper.Viper, urlBase string) (int, string) {
	var finalURL string
	largest := 0
	ret := ""
	strategy := "largest-no-logo"

	if setting.IsSet("strategy") {
		strategy = setting.GetString("strategy")
	}

	logrus.Debugf("use strategy %s", strategy)
	// TODO support manual width selection

	// detect pictures with logo at first because they are in best resolution unless user ask not to
	if strategy == "largest" {
		finalURL = bingBaseURL + urlBase + "_1920x1200.jpg"
		if util.IsReachableLink(finalURL) {
			return 1200, finalURL
		}
	}

	for _, size := range sizeArray {
		finalURL = bingBaseURL + urlBase + "_" + size.size + ".jpg"
		if util.IsReachableLink(finalURL) {
			return size.width, finalURL
		}
	}
	return largest, ret
}

type bingWallpaperChannelProvider int

func (bingWallpaperChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, string, error) {
	var response bingResponse

	historyManager := history.JSONHistoryManagerSingleton
	h, err := historyManager.Load(bingChannelName)
	if err != nil {
		return nil, nil, "", errors.New("loading history failed")
	}
	logrus.Debugf("history of %s channel: %+v", bingChannelName, h)

	// TODO add market as parameter
	if err := util.ReadJson(bingGalleryURL, &response); err != nil {
		return nil, nil, "", err
	}

	logrus.Debugf("JSON loaded %+v", response)

	item := response.Images[0]
	width, finalURL := bingFindFirstFit(setting, item.URLBase)

	logrus.WithField(
		"width", width,
	).WithField(
		"finalUrl", finalURL,
	).WithField(
		"Copyright", item.Copyright,
	).WithField(
		"FullStartDate", item.FullStartDate,
	).Info(
		"picture URL decided",
	)

	// TODO extract following part as util function
	if !setting.GetBool("force") && h.Has(finalURL) {
		logrus.Infoln("bing url alreay exists in history file, ignore.")
		return nil, nil, "", nil
	}

	resp, err := util.GetInType(finalURL, "image/jpeg")
	if err != nil {
		return nil, nil, "", err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, "", err
	}

	raw := bytes.NewReader(bs)
	reader2 := bytes.NewReader(bs)
	img, format, err := image.Decode(reader2)
	if err != nil {
		return raw, nil, "", err
	}
	logrus.WithField("filesize", raw.Len()).Info("wallpaper downloaded")

	h.Mark(finalURL)
	_ = historyManager.Save(h)

	return raw, img, format, nil
}

func init() {
	var me bingWallpaperChannelProvider
	Channels.Register(bingChannelName, me)
}
