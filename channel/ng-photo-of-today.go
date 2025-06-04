package channel

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/PaesslerAG/jsonpath"
	"image"
	_ "image/jpeg" // for jpeg image codec
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	ngChannelName = "ng-photo-of-today"
	ngBaseURL     = "https://www.nationalgeographic.com/photography/photo-of-the-day"
)

type mediaInfo struct {
	// Media type definition based on National Geographic API response
	Img struct {
		Crops []struct {
			Name   string  `mapstructure:"nm"`
			AspRto float64 `mapstructure:"aspRto"`
			URL    string  `mapstructure:"url"`
		} `mapstructure:"crps"`
		RawURL  string `mapstructure:"rt"`
		SrcURL  string `mapstructure:"src"`
		AltText string `mapstructure:"altText"`
		Credit  string `mapstructure:"crdt"`
		Desc    string `mapstructure:"dsc"`
		Title   string `mapstructure:"ttl"`
	} `mapstructure:"img"`
	Slug    string `mapstructure:"slug"`
	Caption struct {
		Credit string `mapstructure:"credit"`
		Text   string `mapstructure:"text"`
		Title  string `mapstructure:"title"`
	} `mapstructure:"caption"`
	Meta struct {
		Title       string `mapstructure:"title"`
		Description string `mapstructure:"description"`
	} `json:"meta"`
}

// extractConfigJSON extracts JSON content between `window['__natgeo__'] = {` and `};`
func extractConfigJSON(r io.Reader) ([]byte, error) {
	const startTag = "window['__natgeo__']={"
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	content := string(data)
	start := strings.Index(content, startTag)
	if start == -1 {
		return nil, fmt.Errorf("config start marker not found")
	}
	start = start + len(startTag) - 1

	// Find the matching closing brace
	level := 0
	for i := start; i < len(content); i++ {
		if content[i] == '{' {
			level++
		} else if content[i] == '}' {
			level--
			if level == 0 {
				return []byte(content[start : i+1]), nil
			}
		}
	}
	return nil, fmt.Errorf("matching config end not found")
}

type ngPoTChannelProvider int

func (ngPoTChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
	var page map[string]interface{}
	historyManager := history.JSONHistoryManagerSingleton
	h, err := historyManager.Load(ngChannelName)
	if err != nil {
		return nil, nil, nil, errors.New("loading history failed")
	}

	logrus.Debugf("history of %s channel: %+v", ngChannelName, h)

	if err := util.ExtractJSON(ngBaseURL, &page, extractConfigJSON); err != nil {
		return nil, nil, nil, err
	}

	mediaSpotlightEdges, err := jsonpath.Get("$..edgs[?(@.cmsType==\"MediaSpotlightContentsTile\")]", page)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get media spotlight edges: %w", err)
	}
	if mediaSpotlightEdges == nil {
		return nil, nil, nil, errors.New("no media spotlight edges found")
	}

	edges, ok := mediaSpotlightEdges.([]interface{})
	if !ok {
		return nil, nil, nil, errors.New("media spotlight edges is not an array")
	}
	if len(edges) == 0 {
		return nil, nil, nil, errors.New("media spotlight edges array is empty")
	}

	logrus.Debugf("%d edges parsed: %#v", len(edges), edges)

	edge0, ok := edges[0].(map[string]interface{})
	if !ok {
		return nil, nil, nil, errors.New("first edge is not an object")
	}

	media, ok := edge0["media"]
	if !ok {
		return nil, nil, nil, errors.New("no media field in first edge")
	}

	var items []mediaInfo
	if err := util.MapToStruct(media, &items); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal media data: %w", err)
	}

	logrus.Debugf("items: %#v", items)

	if len(items) == 0 {
		return nil, nil, nil, errors.New("no picture items found")
	}

	item := items[0]

	meta := &PictureMeta{
		Title:        item.Caption.Title,
		Caption:      item.Caption.Text,
		Credit:       item.Caption.Credit,
		DownloadTime: time.Now().Local(),
		UploadTime:   time.Now().Local(),
	}

	picURL := item.Img.SrcURL

	if picURL == "" {
		return nil, nil, meta, errors.New("no picture URL found")
	}
	base, err := url.Parse(picURL)
	if err != nil {
		return nil, nil, meta, err
	}

	downloadURL, err := url.Parse(picURL)
	if err != nil {
		return nil, nil, meta, err
	}

	finalURL := base.ResolveReference(downloadURL).String()

	logrus.WithField(
		"picURL", picURL,
	).WithField(
		"finalUrl", finalURL,
	).WithField(
		"title", meta.Title,
	).WithField(
		"caption", meta.Caption,
	).Info(
		"picture URL decided",
	)

	if !setting.GetBool("force") && h.Has(finalURL) {
		logrus.Infoln("ngItem url already exists in history file, ignore.")
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
	err = historyManager.Save(h)
	logrus.Warnf("save history error: %v", err)

	return raw, img, meta, nil
}

func init() {
	var me ngPoTChannelProvider
	Channels.Register(ngChannelName, me)
}
