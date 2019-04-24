package channel

import (
	"bytes"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/util"
	"github.com/spf13/viper"
	"image"
	"io/ioutil"
	"net/url"
)

const (
	UNSPLASH_CHANNEL_NAME = "unsplash"
	UNSPLASH_BASE_URL     = "https://api.unsplash.com"
	UNSPLASH_GALLERY_URL  = UNSPLASH_BASE_URL + "/photos/random"
)

type photoItem struct {
	URLs struct {
		Raw string
	}
	Links struct {
		Download string
	}
}

func getClientID() string {
	key := ""
	if key = viper.GetString(UNSPLASH_CHANNEL_NAME + ".key"); key == "" {
		logrus.Fatalf(
			"API access key must be set to %s, create a developer account for API key here: https://unsplash.com/oauth/applications",
			UNSPLASH_CHANNEL_NAME+".key",
		)
	}
	return key
}

func getListQuery() string {
	v := url.Values{
		"client_id": {getClientID()},
	}

	if orientation := viper.GetString(UNSPLASH_CHANNEL_NAME + ".orientation"); orientation != "" {
		v.Set("orientation", orientation)
	}

	if query := viper.GetString(UNSPLASH_CHANNEL_NAME + ".query"); query != "" {
		v.Set("query", query)
	}

	if viper.GetBool(UNSPLASH_CHANNEL_NAME + ".featured") {
		v.Set("featured", "")
	}

	logrus.Debugf("query for photo list: %s", v.Encode())
	return v.Encode()
}

func getPhotoQuery() string {
	v := url.Values{
		"client_id": {getClientID()},
		"fm":        {"jpg"},
		"crop":      {"entropy"},
	}
	if viper.IsSet(UNSPLASH_CHANNEL_NAME + ".strategy") {
		if "by-width" == viper.GetString(UNSPLASH_CHANNEL_NAME+".strategy") {
			if !viper.IsSet(UNSPLASH_CHANNEL_NAME + ".width") {
				logrus.Fatalf("%s must be set to use by-width strategy", UNSPLASH_CHANNEL_NAME+".width")
			} else {
				v.Set("w", viper.GetString(UNSPLASH_CHANNEL_NAME+".width"))
			}
		}
	}

	for key, val := range viper.GetStringMapString(UNSPLASH_CHANNEL_NAME + ".image_parameters") {
		v.Set(key, val)
	}

	logrus.Debugf("query for photo download: %s", v.Encode())
	return v.Encode()
}

type UnsplashWallpaperChannelProvider int

func (UnsplashWallpaperChannelProvider) Download() (*bytes.Reader, image.Image, string, error) {
	if getClientID() == "" {
		return nil, nil, "", fmt.Errorf("unsplash API access key not set")
	}
	query := getListQuery()
	response := photoItem{}
	if err := util.ReadJson(UNSPLASH_GALLERY_URL+"?"+query, &response); err != nil {
		return nil, nil, "", err
	}
	logrus.Debugf("JSON loaded %+v", response)

	// do my best to obey Unsplash API guidelines: https://help.unsplash.com/api-guidelines/more-on-each-guideline/guideline-triggering-a-download
	go func() {
		_, _ = util.GetInType(response.Links.Download, "")
	}()

	finalUrl := response.URLs.Raw + getPhotoQuery()
	resp, err := util.GetInType(finalUrl, "image/jpeg")
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
	var me UnsplashWallpaperChannelProvider
	Channels.Register(UNSPLASH_CHANNEL_NAME, me)
}
