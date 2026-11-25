package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/actor/watermark"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/service"
)

var watermarkCmd = &cobra.Command{
	Use:   "watermark",
	Short: "Inspect and preview watermark layers",
	Long: `Inspect and preview watermark layers.

The layers are edited in the interface's Watermark tab; these commands do from
the command line what it does. Saving layers -- "config import", or the editor
-- applies them from the next refresh.`,
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	exitOn(enc.Encode(v))
}

var watermarkJSON bool

var watermarkFontsCmd = &cobra.Command{
	Use:   "fonts",
	Short: "List the fonts watermark layers can use",
	Long: `List the fonts watermark layers can use.

A layer's font is the file name in the FILE column. Only fonts the renderer can
load are listed: font collections (.ttc) and CFF-flavoured OpenType files are
skipped, and fonts using TrueType features it cannot read (such as Apple's
version 1 kern table) are listed separately, with the reason, on stderr.`,
	Run: func(cmd *cobra.Command, args []string) {
		fonts := watermark.Fonts()
		unsupported := watermark.UnsupportedFonts()
		if watermarkJSON {
			printJSON(map[string]any{"fonts": fonts, "unsupported": unsupported})
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "FAMILY\tSTYLE\tFILE")
		for _, f := range fonts {
			fmt.Fprintf(w, "%s\t%s\t%s\n", f.Family, f.Style, f.File)
		}
		w.Flush()

		// On stderr, so the table stays clean to pipe.
		if len(unsupported) > 0 {
			fmt.Fprintf(os.Stderr, "\n%d installed fonts cannot be used and are not listed:\n", len(unsupported))
			for _, u := range unsupported {
				fmt.Fprintf(os.Stderr, "  %s (%s): %s\n", u.File, u.Family, u.Reason)
			}
		}
	},
}

var watermarkPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List the built-in watermark presets",
	Run: func(cmd *cobra.Command, args []string) {
		presets := watermark.Presets()
		if watermarkJSON {
			printJSON(presets)
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tLAYERS\tDESCRIPTION")
		for _, p := range presets {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", p.ID, p.Name, len(p.Layers), p.Description)
		}
		w.Flush()
	},
}

var watermarkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the saved watermark layers",
	Run: func(cmd *cobra.Command, args []string) {
		layers, err := currentStore().WatermarkLayers()
		exitOn(err)
		if watermarkJSON {
			printJSON(layers)
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "#\tON\tNAME\tANCHOR\tX\tY\tSIZE\tFONT\tTEMPLATE")
		for i, l := range layers {
			on := "no"
			if l.Enabled {
				on = "yes"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%.3g\t%.3g\t%.3g%%\t%s\t%q\n",
				i+1, on, l.Name, l.Anchor, l.X, l.Y, l.Size, l.Font, l.Template)
		}
		w.Flush()
	},
}

var watermarkCaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture the current wallpaper for previewing watermarks on",
	Long: `Capture the current wallpaper for previewing watermarks on.

Previews render onto a captured copy of the current wallpaper as it was
downloaded, before watermarking. A preview captures one by itself when there
is none; capture again after a refresh to preview on the new picture.`,
	Run: func(cmd *cobra.Command, args []string) {
		var base service.PreviewBase
		if c := daemonClient(); c != nil {
			exitOn(c.CapturePreview(&base))
		} else {
			var err error
			base, err = service.NewWatermarks(currentStore()).Capture()
			exitOn(err)
		}
		fmt.Printf("captured %q (%d x %d)\n", base.Title, base.Width, base.Height)
	},
}

var (
	previewLayersFile string
	previewOut        string
	previewDebug      bool
	previewOnDesktop  bool
)

var watermarkPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Render watermark layers onto the current wallpaper",
	Long: `Render watermark layers onto the current wallpaper.

The saved layers are rendered unless --layers names a JSON file holding a list
of layers, as "watermark list --json" prints them. The preview never changes
the wallpaper file or the history: --out writes it to a file, --on-desktop
shows it on the desktop until "goTApaper watermark revert" or the next
refresh. --on-desktop needs a running goTApaper.`,
	Run: func(cmd *cobra.Command, args []string) {
		var layers []store.WatermarkLayer
		if previewLayersFile != "" {
			data, err := os.ReadFile(previewLayersFile)
			exitOn(err)
			exitOn(json.Unmarshal(data, &layers))
		} else {
			var err error
			layers, err = currentStore().WatermarkLayers()
			exitOn(err)
		}
		if previewOut == "" && !previewOnDesktop {
			exitOn(errors.New("say where the preview goes: --out FILE, --on-desktop, or both"))
		}

		var res service.PreviewResult
		var image []byte
		c := daemonClient()
		if c != nil {
			debug := previewDebug
			exitOn(c.RenderPreview(layers, &debug, &res))
			if previewOut != "" {
				var err error
				image, err = c.PreviewImage()
				exitOn(err)
			}
		} else {
			if previewOnDesktop {
				exitOn(errors.New("--on-desktop needs a running goTApaper (gui or daemon)"))
			}
			studio := service.NewWatermarks(currentStore())
			var err error
			res, err = studio.Render(context.Background(), layers, previewDebug)
			exitOn(err)
			image, _, err = studio.Image(0)
			exitOn(err)
		}

		for _, l := range res.Layers {
			switch {
			case !l.Enabled:
			case l.Error != nil:
				where := ""
				if l.Error.Line > 0 {
					where = fmt.Sprintf(" (line %d, column %d)", l.Error.Line, l.Error.Column)
				}
				fmt.Fprintf(os.Stderr, "layer %d: %s error%s: %s\n", l.Index+1, l.Error.Stage, where, l.Error.Message)
			default:
				fmt.Fprintf(os.Stderr, "layer %d: %q\n", l.Index+1, l.Text)
			}
		}

		if previewOut != "" {
			exitOn(os.WriteFile(previewOut, image, 0o644))
			fmt.Printf("preview written to %s (%d x %d)\n", previewOut, res.Width, res.Height)
		}
		if previewOnDesktop {
			exitOn(c.PreviewOnDesktop(nil))
			fmt.Println("previewing on the desktop; \"goTApaper watermark revert\" puts the wallpaper back")
		}
	},
}

var watermarkRevertCmd = &cobra.Command{
	Use:   "revert",
	Short: "Take a watermark preview off the desktop",
	Run: func(cmd *cobra.Command, args []string) {
		c := daemonClient()
		if c == nil {
			exitOn(errors.New("no goTApaper is running, so no preview is on the desktop"))
		}
		exitOn(c.RevertPreview(nil))
		fmt.Println("putting the wallpaper back")
	},
}

func init() {
	for _, c := range []*cobra.Command{watermarkFontsCmd, watermarkPresetsCmd, watermarkListCmd} {
		c.Flags().BoolVar(&watermarkJSON, "json", false, "print JSON")
	}
	f := watermarkPreviewCmd.Flags()
	f.StringVar(&previewLayersFile, "layers", "", "render the layers in this JSON file instead of the saved ones")
	f.StringVarP(&previewOut, "out", "o", "", "write the preview to this file (JPEG)")
	f.BoolVar(&previewDebug, "debug", false, "draw each layer's bounds, box and anchor")
	f.BoolVar(&previewOnDesktop, "on-desktop", false, "show the preview on the desktop until reverted")

	watermarkCmd.AddCommand(
		watermarkFontsCmd, watermarkPresetsCmd, watermarkListCmd,
		watermarkCaptureCmd, watermarkPreviewCmd, watermarkRevertCmd,
	)
	RootCmd.AddCommand(watermarkCmd)
}
