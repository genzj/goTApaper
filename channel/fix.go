package channel

import (
	"bytes"
	"image"
	"io/ioutil"
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
		logrus.WithError(err).Error("cannnot parse fixed upload time")
	}

	finalURL := setting.GetString("url")
	if finalURL == "" {
		logrus.Infoln("blank url, ignore")
		return nil, nil, meta, nil
	}

	resp, err := util.GetInType(finalURL, "image/png")
	if err != nil {
		return nil, nil, meta, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

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

	return raw, img, meta, nil
}

func init() {
	var me fixedPictureProvider
	Channels.Register(fixChannelName, me)
}
