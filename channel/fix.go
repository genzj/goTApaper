package channel

import (
	"bytes"
	"image"
	"net/url"
	"time"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	fixChannelName = "fixed"
)

type fixedPictureProvider int

func (fixedPictureProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
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
	Channels.Register(fixChannelName, me)
}
