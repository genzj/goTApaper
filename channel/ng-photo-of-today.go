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
)

const (
	BASE_URL = "http://www.nationalgeographic.com/photography/photo-of-the-day/_jcr_content/.gallery.json"
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

func findLargest(table sizeTable) (int, string) {
	largest := 0
	ret := ""

	if len(table) == 0 {
		return 0, ret
	}

	for size, url := range table {
		if size > largest {
			ret = url
			largest = size
		}
	}
	return largest, ret
}

type NgPoTChannelProvider int

func (_ *NgPoTChannelProvider) Download(config *config.Config, channelConfig *config.ChannelConfig) (*bytes.Reader, image.Image, error) {
	var toc picTable
	logger := logrus.New()

	if err := util.ReadJson(BASE_URL, &toc); err != nil {
		return nil, nil, err
	}

	if len(toc.Items) < 1 {
		return nil, nil, errors.New("No picture items found")
	}

	item := toc.Items[0]

	width, picurl := findLargest(item.Sizes)
	if picurl == "" {
		return nil, nil, errors.New("No picture URL found")
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
		return nil, nil, err
	}

	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	raw := bytes.NewReader(bs)
	reader2 := bytes.NewReader(bs)
	img, _, err := image.Decode(reader2)
	if err != nil {
		return raw, nil, err
	}
	logger.WithField("filesize", raw.Len()).Info("wallpaper downloaded")

	return raw, img, nil
}
