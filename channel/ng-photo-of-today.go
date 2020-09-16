package channel

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // for jpeg image codec
	"io/ioutil"
	"net/url"
	"strconv"
	"time"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	ngChannelName = "ng-photo-of-today"
	ngBaseURL     = "https://www.nationalgeographic.com/photography/photo-of-the-day/_jcr_content/.gallery.json"
)

type sizeTable map[int]string

type ngRendition struct {
	Width string
	URL   string `json:"uri"`
}

type ngImage struct {
	Title      string
	Caption    string
	Credit     string
	URL        string `json:"uri"`
	Renditions []ngRendition
	Height     int
	Width      int
}

type ngItem struct {
	Image       ngImage `json:"image"`
	PublishDate string
}

type picTable struct {
	Items []ngItem `json:"items"`
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

func findFit(setting *viper.Viper, renditions []ngRendition) (int, string) {
	largest := 0
	ret := ""
	strategy := "largest"

	if setting.IsSet("strategy") {
		strategy = setting.GetString("strategy")
	}

	if len(renditions) == 0 {
		return 0, ret
	}

	for _, rendition := range renditions {
		size, err := strconv.Atoi(rendition.Width)
		if err != nil {
			logrus.WithError(err).Warnf(
				"width in %+v is not an integer, ignore", rendition,
			)
			continue
		}
		if isBetter(setting, strategy, size, largest) {
			ret = rendition.URL
			largest = size
		}
	}
	return largest, ret
}

type ngPoTChannelProvider int

func (ngPoTChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
	var toc picTable

	historyManager := history.JSONHistoryManagerSingleton
	h, err := historyManager.Load(ngChannelName)
	if err != nil {
		return nil, nil, nil, errors.New("loading history failed")
	}

	logrus.Debugf("history of %s channel: %+v", ngChannelName, h)

	if err := util.ReadJSON(ngBaseURL, &toc); err != nil {
		return nil, nil, nil, err
	}
	logrus.Debugf("JSON parsed: %#v", toc)

	if len(toc.Items) < 1 {
		return nil, nil, nil, errors.New("No picture items found")
	}

	item := toc.Items[0].Image

	meta := &PictureMeta{
		Title:        item.Title,
		Caption:      item.Caption,
		Credit:       item.Credit,
		DownloadTime: time.Now(),
		UploadTime:   time.Now(),
	}
	if meta.UploadTime, err = time.ParseInLocation(
		"January 2, 2006", toc.Items[0].PublishDate, time.UTC,
	); err != nil {
		logrus.WithError(err).Warnf(
			"cannot parse publish date of %+v", toc.Items[0],
		)
	} else {
		meta.UploadTime = meta.UploadTime.Local()
	}

	if len(item.Renditions) == 0 && item.Width > 0 {
		logrus.WithField(
			"width", item.Width,
		).WithField(
			"picURL", item.URL,
		).Debug(
			"only one candidate",
		)
		item.Renditions = []ngRendition{
			ngRendition{
				Width: strconv.FormatInt(int64(item.Width), 10), URL: item.URL,
			},
		}
	}

	width, picURL := findFit(setting, item.Renditions)

	if picURL == "" {
		return nil, nil, meta, errors.New("No picture URL found")
	}
	base, err := url.Parse(item.URL)
	if err != nil {
		return nil, nil, meta, err
	}

	downloadURL, err := url.Parse(picURL)
	if err != nil {
		return nil, nil, meta, err
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
		return nil, nil, meta, nil
	}

	resp, err := util.GetInType(finalURL, "image/jpeg")
	if err != nil {
		return nil, nil, meta, err
	}

	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, meta, err
	}

	raw := bytes.NewReader(bs)
	reader2 := bytes.NewReader(bs)
	img, format, err := image.Decode(reader2)
	if err != nil {
		return raw, nil, meta, err
	}
	meta.Format = format
	logrus.WithField("filesize", raw.Len()).Info("wallpaper downloaded")

	h.Mark(finalURL)
	historyManager.Save(h)

	return raw, img, meta, nil
}

func init() {
	var me ngPoTChannelProvider
	Channels.Register(ngChannelName, me)
}
