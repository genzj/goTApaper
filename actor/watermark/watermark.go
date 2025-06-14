package watermark

import (
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/util"
	"image"
	"strings"
	"text/template"

	"github.com/genzj/goTApaper/channel"
	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
)

type watermarkBackgroundSetting struct {
	Paddings    []float64 `mapstructure:"paddings"`
	HThroughout bool      `mapstructure:"h-throughout"`
	VThroughout bool      `mapstructure:"v-throughout"`
	Color       string    `mapstructure:"color"`
}

type watermarkSetting struct {
	Font             string  `mapstructure:"font"`
	Point            float64 `mapstructure:"point"`
	Color            string  `mapstructure:"color"`
	Position         string  `mapstructure:"position"`
	HMargin          float64 `mapstructure:"h-margin"`
	VMargin          float64 `mapstructure:"v-margin"`
	Linespace        float64 `mapstructure:"linespace"`
	Alignment        string  `mapstructure:"alignment"`
	Template         string  `mapstructure:"template"`
	AbsolutePoint    bool    `mapstructure:"absolute-point"`
	AbsolutePosition bool    `mapstructure:"absolute-position"`
	Background       watermarkBackgroundSetting
}

type watermarkGroups struct {
	Watermark []watermarkSetting `mapstructure:"watermark"`
}

// Render watermark to the given image
func Render(im image.Image, meta *channel.PictureMeta) (image.Image, error) {
	type task struct {
		text    string
		setting watermarkSetting
	}
	tasks := []task{}

	groups := watermarkGroups{}
	if err := viper.Unmarshal(&groups); err != nil {
		logrus.WithError(err).Warn(
			"cannot parse watermark settings, skip watermark rendering",
		)
		return im, err
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
		if err := template.Execute(builder, meta); err != nil {
			logrus.WithError(err).Warnf(
				"execute template of %d-th template failed, skip", idx,
			)
			continue
		}
		text := strings.TrimSpace(builder.String())
		logrus.Debugf("render watermark text %#v", text)
		tasks = append(tasks, task{text: text, setting: setting})
	}

	r := newRender(im, watermarkSetting{})
	if viper.GetBool("debug-rendering") {
		minX, minY, maxX, maxY := r.limits()
		w, h := r.size()
		logrus.WithField(
			"w", w,
		).WithField("minX", minX).WithField("minY", minY).WithField("maxX", maxX).WithField("maxY", maxY).WithField("h", h).WithField("bounds", r.ctx.Image().Bounds()).Debugln("dumping original file before rendering")
		radius := 25.0
		r.ctx.Push()

		// Corners
		r.ctx.DrawCircle(0, 0, radius)
		r.ctx.DrawCircle(maxX, maxY, radius)
		r.ctx.SetHexColor("ffff00aa")
		r.ctx.Fill()

		// Bounds
		r.ctx.SetLineWidth(5)
		r.ctx.DrawRectangle(minX, minY, w, h)
		r.ctx.SetHexColor("ffff00aa")
		r.ctx.Stroke()

		wallpaperPath := config.GetWallpaperFileName() + "-before-rendering.jpeg"
		util.SaveImageToJpeg(r.ctx.Image(), wallpaperPath, 75)
		r.ctx.Pop()
	}

	// layer-1, background
	logrus.Debugln("layer-1, background")
	for idx, task := range tasks {
		if task.setting.Background.Color == "" {
			continue
		}
		r.updateSetting(task.setting)
		if err := r.loadFont(); err != nil {
			logrus.Warnf("%d-th watermark ignored due to font loading error", idx)
			continue
		}
		r.renderBackground(task.text)
	}

	// layer-2, text
	logrus.Debugln("layer-2, text")
	for idx, task := range tasks {
		r.updateSetting(task.setting)
		if err := r.loadFont(); err != nil {
			logrus.Warnf("%d-th watermark ignored due to font loading error", idx)
			continue
		}
		r.renderText(task.text)
	}

	// layer-3 debug overlay
	if viper.GetBool("debug-rendering") {
		logrus.Debugln("layer-3, debug overlay")
		for _, task := range tasks {
			r.updateSetting(task.setting)
			postW, postH := r.size()
			logrus.WithField(
				"h", postH,
			).WithField(
				"w", postW,
			).Debug("size after fill")
			minX, minY, _, _ := r.limits()
			r.ctx.SetHexColor("ff0000")
			r.ctx.SetLineWidth(5)
			r.ctx.DrawRectangle(
				minY, minX, postW, postH,
			)
			r.ctx.Stroke()
			r.ctx.DrawLine(
				0, float64(r.ctx.Height())/2,
				float64(r.ctx.Width()), float64(r.ctx.Height())/2,
			)
			r.ctx.DrawLine(
				float64(r.ctx.Width())/2, 0,
				float64(r.ctx.Width())/2, float64(r.ctx.Height()),
			)
			r.ctx.SetDash(10)
			r.ctx.Stroke()
			r.ctx.SetDash()
		}
	}

	return r.ctx.Image(), nil
}
