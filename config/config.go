package config

import (
	"fmt"
	"os"

	yaml "gopkg.in/yaml.v2"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/genzj/goTApaper/util"
)

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

var appName string

// SetAppName for app dir path finding
func SetAppName(name string) {
	appName = name
}

func getAppName() string {
	if appName == "" {
		panic("calling configuration related methods prior to LoadConfig")
	}
	return appName
}

// AppDir returns a path to user config directory
func AppDir() string {
	dir, err := homedir.Expand("~/." + getAppName())
	if err != nil {
		fmt.Println("cannot retrive home dir path")
		fmt.Println(err)
		os.Exit(-1)
	}
	return dir
}

// EnsureAppDir creates app dir if it doesn't exist
func EnsureAppDir() {
	if err := os.MkdirAll(AppDir(), 0755); err != nil {
		fmt.Println("cannot create app dir " + AppDir())
		fmt.Println(err)
		os.Exit(-1)
	}
}

// SaveConfig dump configurations into a disk file
func SaveConfig() error {
	fn := viper.ConfigFileUsed()
	if fn == "" {
		return fmt.Errorf("Configuration file not specified")
	}
	return SaveYaml(fn)

}

// SaveYaml dumps configuration into a YAML file
func SaveYaml(fn string) error {
	f, err := os.Create(fn)
	if err != nil {
		logrus.WithField("filename", fn).Error("cannot open configuration file for writing")
		logrus.Error(err)
		return err
	}
	defer f.Close()

	conf := map[string]interface{}{}

	if err = viper.Unmarshal(&conf); err != nil {
		logrus.Error("cannot dump configuration structure")
		logrus.Error(err)
		return err
	}

	bs, err := yaml.Marshal(conf)
	if err != nil {
		logrus.Error("cannot marshal configuration structure to yaml")
		logrus.Error(err)
		return err
	}

	n, err := f.Write(bs)
	if err != nil {
		logrus.WithField("filename", fn).Errorf("cannot write to file %s", fn)
		logrus.Error(err)
		return err
	}
	logrus.WithField("filename", fn).WithField("n", n).Debug("configuration saved")

	return nil

}

// LoadConfig reads configurations from env and different sources of files
func LoadConfig(cfgFile string) {
	InitDefaultConfig()
	viper.SetEnvPrefix(getAppName())
	viper.SetConfigType("yaml")

	if cfgFile != "" { // enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")                // name of config file (without extension)
		viper.AddConfigPath(".")                     // add current directory as first search path
		viper.AddConfigPath(AppDir())                // add user app path
		viper.AddConfigPath(util.ExecutableFolder()) // add execution file directory as next search path
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
		}).Fatalf("Config File is not readable: %s", err)
	} else {
		logrus.Debugln("No config file found, use default settings")
	}
	initLogger() // intentionally repeat, in case config file updates settings
	logrus.Debugf("%+v", viper.AllSettings())

	Observe("debug", func(_ string, _, _ interface{}) {
		initLogger()
	})
}
