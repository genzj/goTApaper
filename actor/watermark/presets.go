package watermark

import (
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// Preset is a built-in starting point for the editor: a named list of layers.
// Because layers are picture-relative, a preset looks the same on every
// picture and every display.
type Preset struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Layers      []store.WatermarkLayer `json:"layers"`
}

// Presets returns the built-in presets, with fonts resolved to one installed
// here.
func Presets() []Preset {
	font := DefaultFont()
	layer := func(name string, mutate func(*store.WatermarkLayer)) store.WatermarkLayer {
		l := store.NewWatermarkLayer(font)
		l.Name = name
		l.Shadow = store.WatermarkShadow{}
		mutate(&l)
		return l
	}
	softShadow := store.WatermarkShadow{Color: "00000099", DX: 0.06, DY: 0.06}

	presets := []Preset{
		{
			ID: "corner-credit", Name: "Corner credit",
			Description: "A small title and credit in the bottom-right corner.",
			Layers: []store.WatermarkLayer{
				layer("Credit", func(l *store.WatermarkLayer) {
					l.Template = `{{join " · " .Title .Credit}}`
					l.Size = 1.4
					l.Color = "ffffffe6"
					l.Anchor, l.X, l.Y = config.PositionBottomRight, 0.98, 0.97
					l.Alignment = config.AlignRight
					l.Shadow = softShadow
				}),
			},
		},
		{
			ID: "caption-bar", Name: "Caption bar",
			Description: "The title on a translucent bar across the bottom.",
			Layers: []store.WatermarkLayer{
				layer("Caption", func(l *store.WatermarkLayer) {
					l.Template = `{{join " — " .Title .Credit}}`
					l.Size = 1.6
					l.Color = "ffffff"
					l.Anchor, l.X, l.Y = config.PositionBottomCenter, 0.5, 0.965
					l.Alignment = config.AlignCenter
					l.Background = store.WatermarkBackground{
						Color: "00000080", HThroughout: true, Padding: [4]float64{0.7, 0, 0.7, 0},
					}
				}),
			},
		},
		{
			ID: "centred-title", Name: "Centred title",
			Description: "A large outlined title in the middle of the picture.",
			Layers: []store.WatermarkLayer{
				layer("Title", func(l *store.WatermarkLayer) {
					l.Template = "{{upper .Title}}"
					l.Size = 4.5
					l.Color = "ffffffee"
					l.Anchor, l.X, l.Y = config.PositionMiddleCenter, 0.5, 0.5
					l.Alignment = config.AlignCenter
					l.Outline = store.WatermarkOutline{Color: "00000088", Width: 0.04}
				}),
			},
		},
		{
			ID: "photo-card", Name: "Photo card",
			Description: "Title and credit on a light card in the bottom-left corner.",
			Layers: []store.WatermarkLayer{
				layer("Card", func(l *store.WatermarkLayer) {
					l.Template = "{{.Title}}\n{{.Credit}}"
					l.Size = 1.5
					l.Color = "1f1f1f"
					l.Anchor, l.X, l.Y = config.PositionBottomLeft, 0.04, 0.93
					l.Alignment = config.AlignLeft
					l.Background = store.WatermarkBackground{
						Color: "ffffffd9", Padding: [4]float64{0.9, 1.1, 0.9, 1.1},
					}
				}),
			},
		},
		{
			ID: "minimal-date", Name: "Minimal date",
			Description: "Today's date and the title, stacked in the top-right corner.",
			Layers: []store.WatermarkLayer{
				layer("Date", func(l *store.WatermarkLayer) {
					l.Template = `{{date "Monday, January 2" .DownloadTime}}`
					l.Size = 2.4
					l.Color = "ffffff"
					l.Anchor, l.X, l.Y = config.PositionTopRight, 0.97, 0.05
					l.Alignment = config.AlignRight
					l.Shadow = softShadow
				}),
				layer("Title", func(l *store.WatermarkLayer) {
					l.Template = "{{.Title}}"
					l.Size = 1.3
					l.Color = "ffffffcc"
					l.Anchor, l.X, l.Y = config.PositionTopRight, 0.97, 0.085
					l.Alignment = config.AlignRight
					l.Shadow = softShadow
				}),
			},
		},
		{
			ID: "top-ribbon", Name: "Top ribbon",
			Description: "Title and credit on a slim dark ribbon across the top.",
			Layers: []store.WatermarkLayer{
				layer("Ribbon", func(l *store.WatermarkLayer) {
					l.Template = `{{join "   ·   " .Title .Credit}}`
					l.Size = 1.3
					l.Color = "f2f2f2"
					l.Anchor, l.X, l.Y = config.PositionTopLeft, 0.02, 0.012
					l.Alignment = config.AlignLeft
					l.Background = store.WatermarkBackground{
						Color: "000000a6", HThroughout: true, Padding: [4]float64{0.6, 0, 0.6, 0},
					}
				}),
			},
		},
	}

	for _, p := range presets {
		for i := range p.Layers {
			p.Layers[i].SortOrder = i
		}
	}
	return presets
}
