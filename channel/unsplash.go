package channel

import (
	"bytes"
	"fmt"
	"image"
	"io/ioutil"
	"net/url"

	"github.com/sirupsen/logrus"
	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
)

const (
	unsplashChannelName = "unsplash"
	unsplashBaseURL     = "https://api.unsplash.com"
	unsplashGalleryURL  = unsplashBaseURL + "/photos/random"
)

type photoItem struct {
	URLs struct {
		Raw string
	}
	Links struct {
		Download string
	}
}

func getClientID(setting *viper.Viper) string {
	key := ""
	if key = setting.GetString("key"); key == "" {
		logrus.Fatalf(
			"API access key must be set to %s, create a developer account for API key here: https://unsplash.com/oauth/applications",
			unsplashChannelName+".key",
		)
	}
	return key
}

func getListQuery(setting *viper.Viper) string {
	v := url.Values{
		"client_id": {getClientID(setting)},
	}

	if orientation := setting.GetString("orientation"); orientation != "" {
		v.Set("orientation", orientation)
	}

	if query := setting.GetString("query"); query != "" {
		v.Set("query", query)
	}

	if setting.GetBool("featured") {
		v.Set("featured", "")
	}

	logrus.Debugf("query for photo list: %s", v.Encode())
	return v.Encode()
}

func getPhotoQuery(setting *viper.Viper) string {
	v := url.Values{
		"client_id": {getClientID(setting)},
		"fm":        {"jpg"},
		"crop":      {"entropy"},
	}
	if setting.IsSet("strategy") {
		if "by-width" == setting.GetString("strategy") {
			if !setting.IsSet("width") {
				logrus.Fatalf("%s must be set to use by-width strategy", unsplashChannelName+".width")
			} else {
				v.Set("w", setting.GetString("width"))
			}
		}
	}

	for key, val := range setting.GetStringMapString("image_parameters") {
		v.Set(key, val)
	}

	logrus.Debugf("query for photo download: %s", v.Encode())
	return v.Encode()
}

type unsplashWallpaperChannelProvider int

func (unsplashWallpaperChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, string, error) {
	if getClientID(setting) == "" {
		return nil, nil, "", fmt.Errorf("unsplash API access key not set")
	}
	query := getListQuery(setting)
	response := photoItem{}
	if err := util.ReadJson(unsplashGalleryURL+"?"+query, &response); err != nil {
		return nil, nil, "", err
	}
	logrus.Debugf("JSON loaded %+v", response)

	// do my best to obey Unsplash API guidelines:
	// https://help.unsplash.com/api-guidelines/more-on-each-guideline/guideline-triggering-a-download
	go func() {
		resp, err := util.GetInType(response.Links.Download, "")
		if err != nil {
			logrus.Warnf("report download failed: %s", err)
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if response.URLs.Raw == "" {
		logrus.Error(
			"no photo URL received, ensure API secret key is correctly set in the config",
		)
		return nil, nil, "", fmt.Errorf("cannot get photo from unsplash API")
	}

	finalURL := response.URLs.Raw + getPhotoQuery(setting)
	resp, err := util.GetInType(finalURL, "image/jpeg")
	if err != nil {
		return nil, nil, "", err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

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

	return raw, img, format, nil
}

func init() {
	var me unsplashWallpaperChannelProvider
	Channels.Register(unsplashChannelName, me)
}
