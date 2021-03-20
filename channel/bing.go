package channel

import (
	"bytes"
	"errors"
	"image"
	"strings"
	"time"

	"github.com/genzj/goTApaper/config"
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

type sizeArray = []struct {
	size string
}

var fullSizeArray = sizeArray{
	{size: "UHD"},
	{size: "1920x1200"},
	{size: "1920x1080"},
}

var noLogoSizeArray = sizeArray{
	{size: "UHD"},
	{size: "1920x1080"},
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

const bingCreditStarter = '('
const bingCreditStopper = ')'

func splitBingCopyright(copyright string, meta *PictureMeta) {
	offset := strings.IndexFunc(copyright, func(r rune) bool { return r == bingCreditStarter })
	if offset < 0 {
		logrus.Warnf("credit not found in %s", copyright)
		meta.Title = copyright
		meta.Credit = "unknown"
		return
	}

	meta.Title = strings.TrimSpace(copyright[0:offset])

	ending := strings.IndexFunc(copyright[offset:], func(r rune) bool { return r == bingCreditStopper })
	if ending < 0 {
		ending = len(copyright)
	} else {
		ending += offset
	}
	meta.Credit = copyright[offset+1 : ending]
}

func bingFindFirstFit(setting *viper.Viper, urlBase string) string {
	var finalURL string
	var array sizeArray
	ret := ""
	strategy := "largest-no-logo"

	if setting.IsSet("strategy") {
		strategy = setting.GetString("strategy")
	}

	logrus.Debugf("use strategy %s", strategy)
	// TODO support manual width selection

	if strategy == config.LargestNoLogo {
		array = noLogoSizeArray
	} else {
		array = fullSizeArray
	}

	for _, size := range array {
		finalURL = bingBaseURL + urlBase + "_" + size.size + ".jpg"
		if util.IsReachableLink(finalURL) {
			return finalURL
		}
	}
	return ret
}

type bingWallpaperChannelProvider int

func (bingWallpaperChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
	var response bingResponse

	historyManager := history.JSONHistoryManagerSingleton
	h, err := historyManager.Load(bingChannelName)
	if err != nil {
		return nil, nil, nil, errors.New("loading history failed")
	}
	logrus.Debugf("history of %s channel: %+v", bingChannelName, h)

	// TODO add market as parameter
	if err := util.ReadJSON(bingGalleryURL, &response); err != nil {
		return nil, nil, nil, err
	}

	logrus.Debugf("JSON loaded %+v", response)

	item := response.Images[0]
	finalURL := bingFindFirstFit(setting, item.URLBase)

	logrus.WithField(
		"finalUrl", finalURL,
	).WithField(
		"Copyright", item.Copyright,
	).WithField(
		"FullStartDate", item.FullStartDate,
	).Info(
		"picture URL decided",
	)

	// fill metadata
	meta := &PictureMeta{}
	splitBingCopyright(item.Copyright, meta)
	if meta.UploadTime, err = time.ParseInLocation(
		"200601020304", item.FullStartDate, time.UTC,
	); err != nil {
		logrus.Warnf("cannot understand upload time %s", item.FullStartDate)
	} else {
		// use local time so that users can Format directly in watermark
		// templates
		meta.UploadTime = meta.UploadTime.Local()
	}
	meta.DownloadTime = time.Now()

	// TODO extract following part as util function
	if !setting.GetBool("force") && h.Has(finalURL) {
		logrus.Infoln("bing url alreay exists in history file, ignore.")
		return nil, nil, meta, nil
	}

	resp, err := util.GetInType(finalURL, "image/jpeg")
	if err != nil {
		return nil, nil, meta, err
	}
	raw, img, format, err := util.DecodeFromResponse(resp)
	meta.Format = format
	if err != nil {
		return raw, nil, meta, err
	}

	h.Mark(finalURL)
	_ = historyManager.Save(h)

	return raw, img, meta, nil
}

func init() {
	var me bingWallpaperChannelProvider
	Channels.Register(bingChannelName, me)
}
