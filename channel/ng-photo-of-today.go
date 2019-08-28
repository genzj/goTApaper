package channel

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	"io/ioutil"
	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
)

const (
	NG_CHANNEL_NAME = "ng-photo-of-today"
	NG_BASE_URL     = "http://www.nationalgeographic.com/photography/photo-of-the-day/_jcr_content/.gallery.json"
)

type sizeTable map[int]string

type ngItem struct {
	Title       string
	Caption     string
	Credit      string
	URL         string
	Sizes       sizeTable
	PublishDate string
	Height      int
	Width       int
}

type picTable struct {
	Items []ngItem
}

func isBetter(strategy string, size int, lastSize int) bool {
	switch strategy {
	default:
	case config.Unknown:
		logrus.Fatalf("unknown strategy: %s", strategy)
	case config.ByWidth:
		if !viper.IsSet(NG_CHANNEL_NAME + ".width") {
			logrus.Fatalf("%s must be set to use by-width strategy", NG_CHANNEL_NAME+".width")
		}
		return size == viper.GetInt(NG_CHANNEL_NAME+".width")
	case config.LargestNoLogo:
	case config.Largest:
		return size > lastSize
	}
	return false
}

func findFit(table sizeTable) (int, string) {
	largest := 0
	ret := ""
	strategy := "largest"

	if viper.IsSet(NG_CHANNEL_NAME + ".strategy") {
		strategy = viper.GetString(NG_CHANNEL_NAME + ".strategy")
	}

	if len(table) == 0 {
		return 0, ret
	}

	for size, picURL := range table {
		if isBetter(strategy, size, largest) {
			ret = picURL
			largest = size
		}
	}
	return largest, ret
}

type NgPoTChannelProvider int

func (_ NgPoTChannelProvider) Download(force bool) (*bytes.Reader, image.Image, string, error) {
	var toc picTable

	historyManager := history.JsonHistoryManagerSingleton
	h, err := historyManager.Load(NG_CHANNEL_NAME)
	if err != nil {
		return nil, nil, "", errors.New("loading history failed")
	}

	logrus.Debugf("history of %s channel: %+v", NG_CHANNEL_NAME, h)

	if err := util.ReadJson(NG_BASE_URL, &toc); err != nil {
		return nil, nil, "", err
	}

	if len(toc.Items) < 1 {
		return nil, nil, "", errors.New("No picture items found")
	}

	item := toc.Items[0]

	if len(item.Sizes) == 0 && item.Width > 0 {
		logrus.WithField(
			"width", item.Width,
		).WithField(
			"picURL", item.URL,
		).Debug(
			"only one candidate",
		)
		item.Sizes = sizeTable{
			item.Width: item.URL,
		}
	}

	width, picURL := findFit(item.Sizes)

	if picURL == "" {
		return nil, nil, "", errors.New("No picture URL found")
	}
	base, err := url.Parse(item.URL)
	if err != nil {
		return nil, nil, "", err
	}

	downloadUrl, err := url.Parse(picURL)
	if err != nil {
		return nil, nil, "", err
	}

	finalUrl := base.ResolveReference(downloadUrl).String()

	logrus.WithField(
		"width", width,
	).WithField(
		"picURL", picURL,
	).WithField(
		"finalUrl", finalUrl,
	).WithField(
		"title", item.Title,
	).WithField(
		"caption", item.Caption,
	).Info(
		"picture URL decided",
	)

	if !force && h.Has(finalUrl) {
		logrus.Infoln("ngItem url alreay exists in history file, ignore.")
		return nil, nil, "", nil
	}

	resp, err := util.GetInType(finalUrl, "image/jpeg")
	if err != nil {
		return nil, nil, "", err
	}

	defer resp.Body.Close()

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
	historyManager.Save(h)

	return raw, img, format, nil
}

func init() {
	var me NgPoTChannelProvider
	Channels.Register(NG_CHANNEL_NAME, me)
}
