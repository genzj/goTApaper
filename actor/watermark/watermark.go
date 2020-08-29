package watermark

import (
	"image"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
)

type watermarkSetting struct {
	Font      string  `mapstructure:"font"`
	Point     float64 `mapstructure:"point"`
	Color     string  `mapstructure:"color"`
	Position  string  `mapstructure:"position"`
	HPadding  float64 `mapstructure:"hPadding"`
	VPadding  float64 `mapstructure:"vPadding"`
	Linespace float64 `mapstructure:"linespace"`
	Alignment string  `mapstructure:"alignment"`
	Template  string  `mapstructure:"template"`
}

type watermarkGroups struct {
	Watermark []watermarkSetting `mapstructure:"watermark"`
}

type watermarkContext struct {
	Title        string
	Credit       string
	UploadTime   time.Time
	DownloadTime time.Time
}

// Render watermark to the given image
func Render(im image.Image) (image.Image, error) {
	groups := watermarkGroups{}
	if err := viper.Unmarshal(&groups); err != nil {
		logrus.WithError(err).Warn(
			"cannot parse watermark settings, skip watermark rendering",
		)
		return nil, err
	}
	logrus.Debugf("%d watermark to render", len(groups.Watermark))

	for idx, setting := range groups.Watermark {
		logrus.Debugf("start rendering watermark %##v", setting)
		template := template.New("watermark")
		template, err := template.Parse(setting.Template)
		if err != nil {
			logrus.WithError(err).Warnf(
				"parse template of %d-th template failed, skip", idx,
			)
			continue
		}
		logrus.Debugf(
			"parse template of %d-th template successfully: %##v", idx, template,
		)

		builder := &strings.Builder{}
		if err := template.Execute(builder, watermarkContext{
			Title:        "anything",
			Credit:       "ooohhhh",
			UploadTime:   time.Now(),
			DownloadTime: time.Now(),
		}); err != nil {
			logrus.WithError(err).Warnf(
				"execute template of %d-th template failed, skip", idx,
			)
			continue
		}
		text := strings.TrimSpace(builder.String())
		logrus.Debugf("render watermark text %#v", text)
		r := newRender(im, setting)
		if err := r.loadFont(); err != nil {
			logrus.Warnf("%d-th watermark ignored due to font loading error", idx)
			continue
		}
		r.loadColor()
		r.renderText(text)
		im = r.image()
	}

	return im, nil
}
