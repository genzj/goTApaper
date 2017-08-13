package channel

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	"io/ioutil"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
)

const (
	CHANNEL_NAME = "ng-photo-of-today"
	BASE_URL     = "http://www.nationalgeographic.com/photography/photo-of-the-day/_jcr_content/.gallery.json"
)

type sizeTable map[int]string

type item struct {
	Title       string
	Caption     string
	Credit      string
	URL         string
	Sizes       sizeTable
	PublishDate string
}

type picTable struct {
	Items []item
}

func isBetter(strategy string, size int, url string, lastSize int, lastUrl string) bool {
	switch strategy {
	default:
	case config.Unknown:
		logrus.Fatalf("unknown strategy: %s", strategy)
	case config.ByWidth:
		if !viper.IsSet(CHANNEL_NAME + ".width") {
			logrus.Fatalf("%s must be set to use by-width strategy", CHANNEL_NAME+".width")
		}
		return size == viper.GetInt(CHANNEL_NAME+".width")
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

	if viper.IsSet(CHANNEL_NAME + ".strategy") {
		strategy = viper.GetString(CHANNEL_NAME + ".strategy")
	}

	if len(table) == 0 {
		return 0, ret
	}

	for size, url := range table {
		if isBetter(strategy, size, url, largest, ret) {
			ret = url
			largest = size
		}
	}
	return largest, ret
}

type NgPoTChannelProvider int

func (_ NgPoTChannelProvider) Download() (*bytes.Reader, image.Image, string, error) {
	var toc picTable
	logger := logrus.New()

	if err := util.ReadJson(BASE_URL, &toc); err != nil {
		return nil, nil, "", err
	}

	if len(toc.Items) < 1 {
		return nil, nil, "", errors.New("No picture items found")
	}

	item := toc.Items[0]

	width, picurl := findFit(item.Sizes)
	if picurl == "" {
		return nil, nil, "", errors.New("No picture URL found")
	}
	logger.WithField(
		"width", width,
	).WithField(
		"picurl", item.URL+picurl,
	).WithField(
		"title", item.Title,
	).WithField(
		"caption", item.Caption,
	).Info(
		"picture URL decided",
	)

	resp, err := util.GetInType(item.URL+picurl, "image/jpeg")
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
	logger.WithField("filesize", raw.Len()).Info("wallpaper downloaded")

	return raw, img, format, nil
}

func init() {
	var me NgPoTChannelProvider
	Channels.Register(CHANNEL_NAME, me)
}
