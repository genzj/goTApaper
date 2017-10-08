package cmd

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/util"
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

func initLogger() {
	//file, _ := os.OpenFile("logrus.log", os.O_CREATE|os.O_WRONLY, 0666)

	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	//logrus.SetOutput(file)

	if viper.GetBool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugln("Debug log enabled")
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
}

func initConfig() {
	config.InitDefaultConfig()
	viper.SetEnvPrefix(AppName)
	viper.SetConfigType("yaml")

	if cfgFile != "" { // enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")                // name of config file (without extension)
		viper.AddConfigPath(util.ExecutableFolder()) // adding home directory as first search path
	}

	viper.AutomaticEnv() // read in environment variables that match

	initLogger()
	logrus.Debugln("config file searching folder: ", util.ExecutableFolder())
	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		logrus.Debugln("Using config file:", viper.ConfigFileUsed())
	} else if cfgFile != "" {
		logrus.WithFields(logrus.Fields{
			"CfgFile": cfgFile,
		}).Fatalf("Config File is not readable")
	} else {
		logrus.Debugln("No config file found, use default settings")
	}
	initLogger() // intentionally repeat, in case config file updates settings
	logrus.Debugf("%+v", viper.AllSettings())
}

func init() {
	// disable mouse trap, enable start from windows explore
	cobra.MousetrapHelpText = ""
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config-file", "c", "", "config file (default is ./config.yaml in the folder of app executable binary.)")
	RootCmd.PersistentFlags().String("history-file", "", "history file (default is ./history.json in the folder of app executable binary.)")
	viper.BindPFlag("history-file", RootCmd.PersistentFlags().Lookup("history-file"))
	RootCmd.PersistentFlags().StringVar(&lang, "lang", "en-us", "language used for display, in xx-YY format.")
	viper.BindPFlag("language", RootCmd.PersistentFlags().Lookup("lang"))
	RootCmd.PersistentFlags().Bool("debug", false, "enable debug log output")
	viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
}
