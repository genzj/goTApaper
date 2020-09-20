package channel

import (
	"bytes"
	"errors"
	"image"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/history"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type pexelsPhotoObject struct {
	ID             int64             `json:"id"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	URL            string            `json:"url"`
	Photographer   string            `json:"photographer"`
	PhotographerID int64             `json:"photographer_id"`
	Sources        map[string]string `json:"src"`
}

type pexelsCuratedResponse struct {
	Page     int64               `json:"page"`
	PerPage  int64               `json:"per_page"`
	Photos   []pexelsPhotoObject `json:"photos"`
	NextPage string              `json:"next_page"`
}

const (
	pexelsCuratedChannelName   = "pexels-curated"
	pexelsCuratedBaseURL       = "https://api.pexels.com/v1/"
	pexelsCuratedURL           = pexelsCuratedBaseURL + "curated?per_page=1"
	pexelsCuratedPhotoURLField = "original"
)

func extractTitleFromURL(pageURL string) string {
	logger := logrus.WithField("raw", pageURL)
	parsed, err := url.Parse(pageURL)
	if err != nil {
		logger.WithError(err).Warnf("cannot parse URL %s", pageURL)
		return ""
	}

	path := strings.TrimRight(parsed.Path, "/")
	baseIndex := strings.LastIndex(path, "/")
	base := path[baseIndex+1:]
	logger = logger.WithField("path", path).WithField("base", base).WithField("baseIndex", baseIndex)
	logger.Debugf("base found")

	trimmed := strings.TrimRight(base, "-1234567890")
	logger = logrus.WithField("trimmed", trimmed)
	logger.Debugf("photo id trimmed")
	return strings.Title(strings.ReplaceAll(trimmed, "-", " "))
}

type pexelsCuratedChannelProvider int

func (pexelsCuratedChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
	historyManager := history.JSONHistoryManagerSingleton
	h, err := historyManager.Load(pexelsCuratedChannelName)
	if err != nil {
		return nil, nil, nil, errors.New("loading history failed")
	}
	logrus.Debugf("history of %s channel: %+v", pexelsCuratedChannelName, h)

	meta := &PictureMeta{}
	meta.Channel = pexelsCuratedChannelName
	response := pexelsCuratedResponse{}

	req, err := http.NewRequest("GET", pexelsCuratedURL, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	req.Header.Set("Authorization", setting.GetString("key"))

	if err := util.DoAndReadJSON(req, &response); err != nil {
		return nil, nil, nil, err
	}

	logrus.Debugf("pexels curated list read: %#v", response)

	if len(response.Photos) < 1 {
		logrus.Warn("no photo in curated list, ignore pexels curated channel")
		return nil, nil, meta, nil
	}
	photo := response.Photos[0]
	meta.Credit = photo.Photographer
	meta.DownloadTime = time.Now()
	// TODO check if it's possible to extract upload time from photo info page
	meta.UploadTime = meta.DownloadTime
	meta.Title = extractTitleFromURL(photo.URL)

	photoURL := photo.Sources[pexelsCuratedPhotoURLField]
	if photoURL == "" {
		logrus.Warn("no photo url, ignore pexels curated channel")
		return nil, nil, meta, nil
	}

	params := url.Values{}
	params.Add("auto", "compress")
	params.Add("cs", "tinysrgb")
	params.Add("fit", "crop")
	if setting.GetString("strategy") == config.BySize {
		params.Add("h", setting.GetString("height"))
		params.Add("w", setting.GetString("width"))
		params.Add("dpr", setting.GetString("dpr"))
	}
	finalURL := photoURL + "?" + params.Encode()
	logrus.WithField("photo-URL", finalURL).Debug("downloading photo")

	if !setting.GetBool("force") && h.Has(finalURL) {
		logrus.Info("pexels curated photo url alreay exists in history file, ignore.")
		return nil, nil, meta, nil
	}

	resp, err := util.GetInType(finalURL, "image/")
	if err != nil {
		return nil, nil, meta, err
	}
	raw, img, format, err := util.DecodeFromResponse(resp)
	meta.Format = format
	if err != nil {
		return nil, nil, meta, err
	}

	h.Mark(finalURL)
	_ = historyManager.Save(h)

	return raw, img, meta, err
}

func init() {
	var me pexelsCuratedChannelProvider
	Channels.Register(pexelsCuratedChannelName, me)
}
