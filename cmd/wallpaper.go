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
	Short: "Inspect the wallpaper now on the desktop",
}

var wallpaperCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the wallpaper now on the desktop",
	Run: func(cmd *cobra.Command, args []string) {
		current, err := wallpaperService().Current()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if writeImage {
			streamCurrentImage()
			return
		}

		if current.Entry == nil {
			fmt.Println("no wallpaper has been applied yet")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		row := func(label, value string) {
			if value != "" {
				fmt.Fprintf(w, "%s:\t%s\n", label, value)
			}
		}
		row("Title", current.Entry.Title)
		row("Credit", current.Entry.Credit)
		row("Channel", current.Entry.ChannelName)
		row("Applied", time.Unix(current.Entry.DownloadTime, 0).Local().String())
		row("Source", current.Entry.URL)
		row("File", current.Path)
		if current.Exists {
			row("Size", util.HumanBytes(current.Size))
			if current.Width > 0 {
				row("Dimensions", fmt.Sprintf("%d x %d", current.Width, current.Height))
			}
		} else {
			// Worth saying rather than omitting: the metadata describes a
			// picture whose file has since been replaced or removed.
			row("File", current.Path+" (missing)")
		}
		w.Flush()
	},
}

var writeImage bool

func init() {
	wallpaperCurrentCmd.Flags().BoolVar(
		&writeImage, "image", false, "write the wallpaper image to stdout instead of its metadata",
	)
	wallpaperCmd.AddCommand(wallpaperCurrentCmd)
	RootCmd.AddCommand(wallpaperCmd)
}

// streamCurrentImage copies the wallpaper file to stdout, so it can be piped
// to a viewer or redirected to a file.
func streamCurrentImage() {
	file, _, err := wallpaperService().CurrentImage()
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
