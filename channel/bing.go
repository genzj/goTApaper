package channel

import (
	"bytes"
	"errors"
	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
	"image"
	"io/ioutil"
)

const (
	BING_CHANNEL_NAME = "bing-wallpaper"
	BING_BASE_URL     = "https://www.bing.com"
	BING_GALLERY_URL  = BING_BASE_URL + "/HPImageArchive.aspx?format=js&mbl=1&idx=-1&n=5"
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

func bingFindFirstFit(urlBase string) (int, string) {
	var finalUrl string
	largest := 0
	ret := ""
	strategy := "largest-no-logo"

	if viper.IsSet(BING_CHANNEL_NAME + ".strategy") {
		strategy = viper.GetString(BING_CHANNEL_NAME + ".strategy")
	}

	logrus.Debugf("use strategy %s", strategy)
	// TODO support manual width selection

	// detect pictures with logo at first because they are in best resolution unless user ask not to
	if strategy == "largest" {
		finalUrl = BING_BASE_URL + urlBase + "_1920x1200.jpg"
		if util.IsReachableLink(finalUrl) {
			return 1200, finalUrl
		}
	}

	for _, size := range sizeArray {
		finalUrl = BING_BASE_URL + urlBase + "_" + size.size + ".jpg"
		if util.IsReachableLink(finalUrl) {
			return size.width, finalUrl
		}
	}
	return largest, ret
}

type BingWallpaperChannelProvider int

func (BingWallpaperChannelProvider) Download() (*bytes.Reader, image.Image, string, error) {
	var response bingResponse

	historyManager := history.JsonHistoryManagerSingleton
	h, err := historyManager.Load(BING_CHANNEL_NAME)
	if err != nil {
		return nil, nil, "", errors.New("loading history failed")
	}
	logrus.Debugf("history of %s channel: %+v", BING_CHANNEL_NAME, h)

	// TODO add market as parameter
	if err := util.ReadJson(BING_GALLERY_URL, &response); err != nil {
		return nil, nil, "", err
	}

	logrus.Debugf("JSON loaded %+v", response)

	item := response.Images[0]
	width, finalUrl := bingFindFirstFit(item.URLBase)

	logrus.WithField(
		"width", width,
	).WithField(
		"finalUrl", finalUrl,
	).WithField(
		"Copyright", item.Copyright,
	).WithField(
		"FullStartDate", item.FullStartDate,
	).Info(
		"picture URL decided",
	)

	// TODO extract following part as util function
	if h.Has(finalUrl) {
		logrus.Infoln("bing url alreay exists in history file, ignore.")
		return nil, nil, "", nil
	}

	resp, err := util.GetInType(finalUrl, "image/jpeg")
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

	h.Mark(finalUrl)
	_ = historyManager.Save(h)

	return raw, img, format, nil
}

func init() {
	var me BingWallpaperChannelProvider
	Channels.Register(BING_CHANNEL_NAME, me)
}
