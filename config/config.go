package config

import (
	"fmt"
	"os"

	yaml "gopkg.in/yaml.v2"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func IsSet(key string) bool {
	return viper.IsSet(key)
}

func IsLeaf(key string) bool {
	return IsSet(key) && len(viper.GetStringMap(key)) == 0
}

func SaveConfig() error {
	if fn := viper.ConfigFileUsed(); fn == "" {
		return fmt.Errorf("Using default configuration file")
	} else {
		return SaveYaml(fn)
	}
}

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

	if n, err := f.Write(bs); err != nil {
		logrus.WithField("filename", fn).Errorf("cannot write to file %s", fn)
		logrus.Error(err)
		return err
	} else {
		logrus.WithField("filename", fn).WithField("n", n).Debug("configuration saved")
	}

	return nil

}
