package cmd

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/util"
)

var wallpaperCmd = &cobra.Command{
	Use:   "wallpaper",
	Short: "Inspect the wallpaper goTApaper applied",
}

var wallpaperLastAppliedCmd = &cobra.Command{
	Use:   "last-applied",
	Short: "Show the wallpaper goTApaper applied most recently",
	Long: `Show the wallpaper goTApaper applied most recently.

This is what goTApaper last set, not necessarily what is on the desktop now:
it cannot see the desktop, and the wallpaper may have been changed by hand
since.`,
	Run: func(cmd *cobra.Command, args []string) {
		last, err := wallpaperService().LastApplied()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if writeImage {
			streamLastAppliedImage()
			return
		}

		if last.Entry == nil {
			fmt.Println("no wallpaper has been applied yet")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		row := func(label, value string) {
			if value != "" {
				fmt.Fprintf(w, "%s:\t%s\n", label, value)
			}
		}
		row("Title", last.Entry.Title)
		row("Credit", last.Entry.Credit)
		row("Channel", last.Entry.ChannelName)
		row("Applied", time.Unix(last.Entry.DownloadTime, 0).Local().String())
		row("Source", last.Entry.URL)
		row("File", last.Path)
		if last.Exists {
			row("Size", util.HumanBytes(last.Size))
			if last.Width > 0 {
				row("Dimensions", fmt.Sprintf("%d x %d", last.Width, last.Height))
			}
		} else {
			// Worth saying rather than omitting: the metadata describes a
			// picture whose file has since been replaced or removed.
			row("File", last.Path+" (missing)")
		}
		w.Flush()
	},
}

var writeImage bool

func init() {
	wallpaperLastAppliedCmd.Flags().BoolVar(
		&writeImage, "image", false, "write the wallpaper image to stdout instead of its metadata",
	)
	wallpaperCmd.AddCommand(wallpaperLastAppliedCmd)
	RootCmd.AddCommand(wallpaperCmd)
}

// streamLastAppliedImage copies the wallpaper file to stdout, so it can be piped
// to a viewer or redirected to a file.
func streamLastAppliedImage() {
	file, _, err := wallpaperService().LastAppliedImage()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()

	if _, err := io.Copy(os.Stdout, file); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
