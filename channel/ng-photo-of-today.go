package channel

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // for jpeg image codec
	"io/ioutil"
	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
)

const (
	ngChannelName = "ng-photo-of-today"
	ngBaseURL     = "http://www.nationalgeographic.com/photography/photo-of-the-day/_jcr_content/.gallery.json"
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

func isBetter(setting *viper.Viper, strategy string, size int, lastSize int) bool {
	switch strategy {
	default:
	case config.Unknown:
		logrus.Fatalf("unknown strategy: %s", strategy)
	case config.ByWidth:
		if !setting.IsSet("width") {
			logrus.Fatalf("%s must be set to use by-width strategy", ngChannelName+".width")
		}
		return size == setting.GetInt("width")
	case config.LargestNoLogo:
	case config.Largest:
		return size > lastSize
	}
	return false
}

func findFit(setting *viper.Viper, table sizeTable) (int, string) {
	largest := 0
	ret := ""
	strategy := "largest"

	if setting.IsSet("strategy") {
		strategy = setting.GetString("strategy")
	}

	if len(table) == 0 {
		return 0, ret
	}

	for size, picURL := range table {
		if isBetter(setting, strategy, size, largest) {
			ret = picURL
			largest = size
		}
	}
	return largest, ret
}

type ngPoTChannelProvider int

func (ngPoTChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, string, error) {
	var toc picTable

	historyManager := history.JsonHistoryManagerSingleton
	h, err := historyManager.Load(ngChannelName)
	if err != nil {
		return nil, nil, "", errors.New("loading history failed")
	}

	logrus.Debugf("history of %s channel: %+v", ngChannelName, h)

	if err := util.ReadJson(ngBaseURL, &toc); err != nil {
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

	width, picURL := findFit(setting, item.Sizes)

	if picURL == "" {
		return nil, nil, "", errors.New("No picture URL found")
	}
	base, err := url.Parse(item.URL)
	if err != nil {
		return nil, nil, "", err
	}

	downloadURL, err := url.Parse(picURL)
	if err != nil {
		return nil, nil, "", err
	}

	finalURL := base.ResolveReference(downloadURL).String()

	logrus.WithField(
		"width", width,
	).WithField(
		"picURL", picURL,
	).WithField(
		"finalUrl", finalURL,
	).WithField(
		"title", item.Title,
	).WithField(
		"caption", item.Caption,
	).Info(
		"picture URL decided",
	)

	if !setting.GetBool("force") && h.Has(finalURL) {
		logrus.Infoln("ngItem url alreay exists in history file, ignore.")
		return nil, nil, "", nil
	}

	resp, err := util.GetInType(finalURL, "image/jpeg")
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

	h.Mark(finalURL)
	historyManager.Save(h)

	return raw, img, format, nil
}

func init() {
	var me ngPoTChannelProvider
	Channels.Register(ngChannelName, me)
}
