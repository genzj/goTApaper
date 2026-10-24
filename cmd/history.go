package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/history"
	"github.com/genzj/goTApaper/service"
	"github.com/genzj/goTApaper/util"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Browse the wallpapers that have been applied",
	Long: `Browse the wallpapers that have been applied.

This is the browsable log, one entry per wallpaper. It is distinct from
"config history", which shows the single most recent download per channel used
to avoid downloading the same picture twice.`,
}

var historyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the wallpapers that have been applied, newest first",
	Run: func(cmd *cobra.Command, args []string) {
		page, err := wallpaperService().WallpaperHistory(historyLimit, historyOffset)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if len(page.Entries) == 0 {
			fmt.Println("no wallpapers have been applied yet")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tAPPLIED\tCHANNEL\tTITLE\tTHUMBNAIL")
		for _, e := range page.Entries {
			thumb := "-"
			if e.ThumbPath != "" {
				thumb = "yes"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
				e.ID,
				time.Unix(e.DownloadTime, 0).Local().Format("2006-01-02 15:04"),
				e.ChannelName,
				truncate(e.Title, 50),
				thumb,
			)
		}
		w.Flush()

		shown := page.Offset + len(page.Entries)
		if page.Total > shown {
			fmt.Printf("\nshowing %d-%d of %d; use --offset %d for more\n",
				page.Offset+1, shown, page.Total, shown)
		}
	},
}

var historyShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show everything recorded about one wallpaper",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := mustParseID(args[0])

		entry, err := wallpaperService().WallpaperHistoryEntry(id)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		row := func(label, value string) {
			if value != "" {
				fmt.Fprintf(w, "%s:\t%s\n", label, value)
			}
		}
		row("ID", fmt.Sprintf("%d", entry.ID))
		row("Channel", entry.ChannelName)
		row("Type", entry.ChannelType)
		row("Title", entry.Title)
		row("Caption", entry.Caption)
		row("Credit", entry.Credit)
		row("Format", entry.Format)
		row("Applied", time.Unix(entry.DownloadTime, 0).Local().String())
		if entry.UploadTime != 0 {
			row("Uploaded", time.Unix(entry.UploadTime, 0).Local().String())
		}
		row("URL", entry.URL)
		row("Wallpaper file", entry.FilePath)
		row("Thumbnail", entry.ThumbPath)
		w.Flush()
	},
}

var historyReapplyCmd = &cobra.Command{
	Use:   "reapply <id>",
	Short: "Download a recorded wallpaper again and set it",
	Long: `Download a recorded wallpaper again and set it.

The picture is fetched from the URL the history kept, not from the channel that
produced it, so this re-applies that exact wallpaper rather than whatever the
provider is serving now. Some providers expire their links, in which case the
download fails and the current wallpaper is left alone.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := mustParseID(args[0])

		// Routed through the daemon when one is running, so the picture is
		// applied by the process that owns the tray state and the database.
		if client := daemonClient(); client != nil {
			var result service.RefreshResult
			if err := client.Reapply(id, &result); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			reportReapplied(result)
			return
		}

		entry, err := currentStore().WallpaperHistoryEntryByID(id)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		meta, err := reapply(entry)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		reportReapplied(service.RefreshResult{
			Channel: meta.ChannelKey, Type: meta.Channel,
			Title: meta.Title, Credit: meta.Credit,
		})
	},
}

var historyPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Enforce the history limit and reclaim unreferenced thumbnails",
	Long: fmt.Sprintf(`Enforce the history limit and reclaim unreferenced thumbnails.

Entries beyond %s are removed, oldest first, along with their thumbnails. Any
thumbnail file no entry refers to is also reclaimed, which is what cleans up
after a crash, a deleted channel, or thumbnails being turned off.

This runs automatically after every refresh; the command is for doing it now.`,
		config.KeyHistoryMaxEntries),
	Run: func(cmd *cobra.Command, args []string) {
		result, err := wallpaperService().PruneHistory(pruneKeep, pruneDryRun)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		reportPruned(result)
	},
}

var historyDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Remove one history entry and its thumbnail",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := mustParseID(args[0])
		if err := wallpaperService().DeleteWallpaperHistoryEntry(id); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("removed history entry %d\n", id)
	},
}

var (
	historyLimit  int
	historyOffset int
	pruneKeep     int
	pruneDryRun   bool
)

func init() {
	historyListCmd.Flags().IntVarP(
		&historyLimit, "limit", "n", service.DefaultHistoryPageSize, "entries to show",
	)
	historyListCmd.Flags().IntVar(
		&historyOffset, "offset", 0, "skip this many entries",
	)

	historyPruneCmd.Flags().IntVar(
		&pruneKeep, "keep", 0,
		"entries to keep; 0 uses the configured "+config.KeyHistoryMaxEntries+
			", a negative value keeps everything",
	)
	historyPruneCmd.Flags().BoolVar(
		&pruneDryRun, "dry-run", false, "report what would be removed without removing it",
	)

	historyCmd.AddCommand(
		historyListCmd, historyShowCmd, historyReapplyCmd,
		historyPruneCmd, historyDeleteCmd,
	)
	RootCmd.AddCommand(historyCmd)
}

// wallpaperService builds the wallpaper service over the local store.
//
// Reads go straight to the database rather than through the daemon: they take
// no lock worth contending and work with nothing running. Writes that the
// daemon should react to route through the client instead.
func wallpaperService() *service.Wallpaper {
	return service.NewWallpaper(currentStore(), refresh, reapply)
}

func reportReapplied(result service.RefreshResult) {
	if result.Title != "" {
		fmt.Printf("applied %q", result.Title)
		if result.Credit != "" {
			fmt.Printf(" by %s", result.Credit)
		}
		fmt.Println()
		return
	}
	fmt.Println("wallpaper applied")
}

func reportPruned(result history.SweepResult) {
	verb := "removed"
	if result.DryRun {
		verb = "would remove"
	}
	fmt.Printf("%s %d entries and %d thumbnail files (%s)\n",
		verb, result.RowsRemoved, result.FilesRemoved,
		util.HumanBytes(result.BytesReclaimed))
}

func mustParseID(raw string) int64 {
	var id int64
	if _, err := fmt.Sscanf(raw, "%d", &id); err != nil || id <= 0 {
		fmt.Fprintf(os.Stderr, "invalid history id %q\n", raw)
		os.Exit(1)
	}
	return id
}

// truncate shortens a value for a table column, marking that it was cut.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
