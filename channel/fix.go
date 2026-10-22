package channel

import (
	"bytes"
	"image"
	"net/url"
	"time"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
)

const (
	fixChannelName = "fixed"
)

type fixedPictureProvider int

func (fixedPictureProvider) Download(setting Settings) (*bytes.Reader, image.Image, *PictureMeta, error) {
	// fill metadata
	meta := &PictureMeta{}
	meta.DownloadTime = time.Now()
	meta.Channel = fixChannelName
	meta.ChannelKey = fixChannelName
	meta.Caption = setting.GetString("meta.caption")
	meta.Credit = setting.GetString("meta.credit")
	meta.Title = setting.GetString("meta.title")
	if uploadTime, err := time.ParseInLocation("200601020304", setting.GetString("meta.upload-time"), time.UTC); err == nil {
		meta.UploadTime = uploadTime
	} else {
		logrus.WithError(err).Error("cannot parse fixed upload time")
	}

	finalURL := setting.GetString("url")
	if finalURL == "" {
		logrus.Infoln("blank url, ignore")
		return nil, nil, meta, nil
	}

	parsedURL, err := url.ParseRequestURI(finalURL)
	if err != nil {
		return nil, nil, meta, err
	}

	if parsedURL.Scheme == "file" {
		finalURL = parsedURL.Path
		raw, img, format, err := util.DecodeFromFile(finalURL)
		meta.Format = format
		return raw, img, meta, err
	} else {
		resp, err := util.GetInType(finalURL, "image/")
		if err != nil {
			return nil, nil, meta, err
		}

		raw, img, format, err := util.DecodeFromResponse(resp)
		meta.Format = format
		return raw, img, meta, err
	}
}

func init() {
	var me fixedPictureProvider
	Channels.RegisterWithSchema(fixChannelName, me, OptionSchema{
		Options: []Option{
			{
				Key: "url", Label: "Picture URL", Kind: OptionString, Required: true,
				Description: "http(s) URL, or a file:// path to a local picture",
			},
			// The metadata is supplied by hand here because a fixed picture
			// has no provider to report a title or credit. These are read with
			// dotted keys, so the store's option lookup must descend into
			// nested objects.
			{
				Key: "meta.title", Label: "Title", Kind: OptionString,
				Description: "title used in watermark templates",
			},
			{
				Key: "meta.caption", Label: "Caption", Kind: OptionString,
				Description: "caption used in watermark templates",
			},
			{
				Key: "meta.credit", Label: "Credit", Kind: OptionString,
				Description: "credit or uploader name used in watermark templates",
			},
			{
				Key: "meta.upload-time", Label: "Upload time", Kind: OptionString,
				Description: "upload time in YYYYMMDDHHmm form, interpreted as UTC",
			},
		},
	})
}
