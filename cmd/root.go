package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/config"
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
	// Retired settings: nothing ever read them. Their flags stay so scripts
	// and login items passing them keep working. Every retired setting was a
	// string, so each takes a value.
	for _, name := range config.RetiredFlags() {
		RootCmd.PersistentFlags().String(name, "", "")
		_ = RootCmd.PersistentFlags().MarkDeprecated(name, "it has no effect and will be removed")
	}
	RootCmd.PersistentFlags().Bool("debug", false, "enable debug log output")
	RootCmd.PersistentFlags().Bool(
		"debug-rendering", false, "enable debug mode in watermark rendering",
	)
}
