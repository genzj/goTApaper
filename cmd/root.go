package cmd

import (
	"fmt"
	"os"

	"github.com/genzj/goTApaper/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// AppName states name of the project
const AppName string = "goTApaper"

var cfgFile string
var lang string

// RootCmd is the entry of whole program
var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "A Go application downloading latest wallpaper from different providers.",
	Long: `Download the wallpaper from different providers and set it your desktop. Also
integrates a WEB UI for configuration.`,
}

//Execute adds all child commands to the root command sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func initConfig() {
	config.SetAppName(AppName)
	config.EnsureAppDir()
	config.LoadConfig(cfgFile)
}

func init() {
	// disable mouse trap, enable start from windows explore
	cobra.MousetrapHelpText = ""
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config-file", "c", "", "config file (default is the first config.yaml found in ~/.goTApaper or the folder of app executable binary.)")
	RootCmd.PersistentFlags().String("history-file", "", "history file (default is ~/.goTApaper/history.json)")
	viper.BindPFlag("history-file", RootCmd.PersistentFlags().Lookup("history-file"))
	RootCmd.PersistentFlags().StringVar(&lang, "lang", "en-us", "language used for display, in xx-YY format.")
	viper.BindPFlag("language", RootCmd.PersistentFlags().Lookup("lang"))
	RootCmd.PersistentFlags().Bool("debug", false, "enable debug log output")
	viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
	RootCmd.PersistentFlags().Bool("debug-rendering", false, "enable debug mode in watermark rendering")
	viper.BindPFlag("debug-rendering", RootCmd.PersistentFlags().Lookup("debug-rendering"))
}
