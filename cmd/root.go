package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// AppName states name of the project
const AppName string = "goTApaper"

var appDir string

// RootCmd is the entry of whole program
var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "A Go application downloading latest wallpaper from different providers.",
	Long: `Download the wallpaper from different providers and set it your desktop. Also
integrates a WEB UI for configuration.`,
}

// Execute adds all child commands to the root command sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func init() {
	// disable mouse trap, enable start from windows explore
	cobra.MousetrapHelpText = ""
	cobra.OnInitialize(loadConfig)

	// Flags are read in cmd/bootstrap.go and pushed into the provider's
	// override layer, so they affect this run only. They are deliberately not
	// bound to the store: a flag must never rewrite the stored configuration.
	RootCmd.PersistentFlags().StringVar(
		&appDir, "app-dir", "",
		"application directory holding the configuration database "+
			"(default ~/.goTApaper, or $GOTAPAPER_APP_DIR)",
	)
	RootCmd.PersistentFlags().String(
		"history-file", "", "legacy history file, used only when importing",
	)
	RootCmd.PersistentFlags().String(
		"lang", "en-us", "language used for display, in xx-YY format.",
	)
	RootCmd.PersistentFlags().Bool("debug", false, "enable debug log output")
	RootCmd.PersistentFlags().Bool(
		"debug-rendering", false, "enable debug mode in watermark rendering",
	)
}
