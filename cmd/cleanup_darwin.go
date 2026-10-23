//go:build darwin
// +build darwin

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/actor/setter"
	"github.com/genzj/goTApaper/config"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Reclaim the disk space macOS keeps for past wallpapers",
	Long: `Remove the copies and index entries macOS accumulates every time a
wallpaper is set.

Each distinct image path handed to the system is remembered forever as an entry
in the "Your Photos" row of System Settings, together with a full-size copy of
the file in the wallpaper extension's own cache. Neither is ever pruned, so a
long-running goTApaper leaves gigabytes behind.

By default only entries goTApaper created are removed. Always worth a --dry-run
first, especially before --wipe-all-photos.`,
	Run: runCleanup,
}

func runCleanup(cmd *cobra.Command, _ []string) {
	flags := cmd.Flags()
	dryRun, _ := flags.GetBool("dry-run")
	allPhotos, _ := flags.GetBool("wipe-all-photos")
	restartAgent, _ := flags.GetBool("restart-agent")
	assumeYes, _ := flags.GetBool("yes")

	appDir := config.AppDir()

	// Plan first and show it, whether or not this is a dry run, so the user
	// sees exactly what is about to happen rather than only what happened.
	plan, err := setter.PruneWallpaperCaches(setter.PruneOptions{
		AppDir:    appDir,
		AllPhotos: allPhotos,
		DryRun:    true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	printPrunePlan(plan, allPhotos)

	if len(plan.Entries) == 0 && len(plan.Renders) == 0 && len(plan.OrphanCopies) == 0 {
		fmt.Println("Nothing to clean up.")
		return
	}

	if dryRun {
		fmt.Println("\nDry run: nothing was removed.")
		return
	}

	// Only the all-photos case needs a prompt. The default case removes
	// goTApaper's own leftovers, which it would also have removed on its own
	// during a refresh.
	if allPhotos && !assumeYes && !confirm(unownedCount(plan)) {
		fmt.Println("Cancelled.")
		return
	}

	stats, err := setter.PruneWallpaperCaches(setter.PruneOptions{
		AppDir:       appDir,
		AllPhotos:    allPhotos,
		RestartAgent: restartAgent,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf(
		"\nRemoved %d picker %s, %d rendered %s and %d unreferenced %s, reclaiming %s.\n",
		len(stats.Entries), entryWord(len(stats.Entries)),
		len(stats.Renders), bitmapWord(len(stats.Renders)),
		len(stats.OrphanCopies), copyWord(len(stats.OrphanCopies)),
		setter.HumanBytes(stats.TotalBytes),
	)
}

func printPrunePlan(plan setter.PruneStats, allPhotos bool) {
	total := len(plan.Entries) + plan.Kept + plan.KeptCurrent
	fmt.Printf("\"Your Photos\" entries to remove (%d of %d):\n", len(plan.Entries), total)

	for _, e := range plan.Entries {
		owner := "other"
		if e.Owned {
			owner = "goTApaper"
		}
		fmt.Printf("  [%-9s] %s\n", owner, setter.DescribeEntry(e))
		if len(e.Renders) > 0 {
			fmt.Printf(
				"              + %d rendered %s (%s)\n",
				len(e.Renders), bitmapWord(len(e.Renders)), setter.HumanBytes(e.RenderBytes),
			)
		}
	}
	if len(plan.Entries) == 0 {
		fmt.Println("  (none)")
	}

	fmt.Printf(
		"\nRendered bitmaps to remove: %d (%s)\n",
		len(plan.Renders), setter.HumanBytes(plan.RenderBytes),
	)
	if n := len(plan.OrphanRenders); n > 0 {
		fmt.Printf(
			"  including %d matching no remaining entry (%s)\n",
			n, setter.HumanBytes(plan.OrphanBytes),
		)
	}
	if plan.RendersKept > 0 {
		// Said explicitly, because with one wallpaper in play the only bitmap
		// on disk belongs to the current wallpaper and the count above reads
		// as zero. Without this line that looks like the pruner missed it.
		fmt.Printf(
			"  keeping %d belonging to wallpapers that stay\n", plan.RendersKept,
		)
	}

	if n := len(plan.OrphanCopies); n > 0 {
		fmt.Printf(
			"\nUnreferenced cached copies to remove: %d (%s)\n",
			n, setter.HumanBytes(plan.OrphanCopyBytes),
		)
	}

	if plan.Kept > 0 {
		fmt.Printf("Keeping %d %s created outside goTApaper.\n", plan.Kept, entryWord(plan.Kept))
	}
	if plan.KeptCurrent > 0 {
		fmt.Printf(
			"Keeping %d %s describing the wallpaper on screen.\n",
			plan.KeptCurrent, entryWord(plan.KeptCurrent),
		)
	}
	if allPhotos {
		if n := unownedCount(plan); n > 0 {
			fmt.Printf(
				"\nWARNING: %d of the entries above were not created by goTApaper.\n"+
					"         They are photos added by hand in System Settings and cannot be restored.\n",
				n,
			)
		}
	}
	fmt.Printf("\nTotal reclaimed: %s\n", setter.HumanBytes(plan.TotalBytes))
}

func entryWord(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}

func bitmapWord(n int) string {
	if n == 1 {
		return "bitmap"
	}
	return "bitmaps"
}

func copyWord(n int) string {
	if n == 1 {
		return "copy"
	}
	return "copies"
}

func unownedCount(plan setter.PruneStats) int {
	n := 0
	for _, e := range plan.Entries {
		if !e.Owned {
			n++
		}
	}
	return n
}

func confirm(unowned int) bool {
	fmt.Printf("\nRemove all of the above, including %d entries you added yourself? [y/N] ", unowned)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func init() {
	flags := cleanupCmd.Flags()
	flags.BoolP("dry-run", "n", false, "list what would be removed and change nothing")
	flags.Bool(
		"wipe-all-photos", false,
		"also remove entries goTApaper did not create, including photos added by hand",
	)
	flags.Bool(
		"restart-agent", false,
		"restart the wallpaper agent afterwards; briefly blanks the desktop",
	)
	flags.BoolP("yes", "y", false, "do not ask for confirmation")

	RootCmd.AddCommand(cleanupCmd)
}
