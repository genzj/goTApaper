package channel

import (
	"bytes"
	"fmt"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"image"
	"net/url"
	"time"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	unsplashChannelName = "unsplash"
	unsplashBaseURL     = "https://api.unsplash.com"
	unsplashGalleryURL  = unsplashBaseURL + "/photos/random"
)

type photoItem struct {
	UpdatedAt      string `json:"updated_at"`
	Description    string
	AltDescription string `json:"alt_description"`

	User struct {
		Name string
	}
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

func (unsplashWallpaperChannelProvider) Download(setting *viper.Viper) (*bytes.Reader, image.Image, *PictureMeta, error) {
	if getClientID(setting) == "" {
		return nil, nil, nil, fmt.Errorf("unsplash API access key not set")
	}
	query := getListQuery(setting)
	response := photoItem{}
	if err := util.ReadJSON(unsplashGalleryURL+"?"+query, &response); err != nil {
		return nil, nil, nil, err
	}
	logrus.Debugf("JSON loaded %+v", response)

	meta := &PictureMeta{
		Title:        response.Description,
		Credit:       response.User.Name,
		DownloadTime: time.Now(),
		UploadTime:   time.Now(),
	}
	if meta.Title == "" {
		meta.Title = response.AltDescription
	}
	meta.Title = cases.Title(language.Und).String(meta.Title)

	var err error
	if meta.UploadTime, err = time.Parse(
		time.RFC3339, response.UpdatedAt,
	); err != nil {
		logrus.WithError(err).Warnf(
			"cannot parse publish date of %+v", response,
		)
	} else {
		meta.UploadTime = meta.UploadTime.Local()
		logrus.Debugf("parsed time: %s", meta.UploadTime.Format(time.RFC3339))
	}

	// do my best to obey Unsplash API guidelines:
	// https://help.unsplash.com/api-guidelines/more-on-each-guideline/guideline-triggering-a-download
	go func() {
		resp, err := util.Get(response.Links.Download)
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
		return nil, nil, meta, fmt.Errorf("cannot get photo from unsplash API")
	}

	finalURL := response.URLs.Raw + getPhotoQuery(setting)
	resp, err := util.GetInType(finalURL, "image/jpeg")
	if err != nil {
		return nil, nil, meta, err
	}
	raw, img, format, err := util.DecodeFromResponse(resp)
	meta.Format = format
	return raw, img, meta, err
}

func init() {
	var me unsplashWallpaperChannelProvider
	Channels.Register(unsplashChannelName, me)
}
