package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/data"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var installMode bool

func init() {
	generateConfigCmd.PersistentFlags().BoolVarP(
		&installMode,
		"install", "i", false,
		"write an example configuration file into ~/.goTApaper",
	)
	RootCmd.AddCommand(generateConfigCmd)
}

var generateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Write sample configuration file to stdout",
	Long:  `Write sample configuration file to stdout`,
	Run: func(cmd *cobra.Command, args []string) {
		var out *os.File

		if installMode {
			target := path.Join(config.AppDir(), "config.yaml")
			l := logrus.WithField(
				"config", target,
			)

			stat, err := os.Stat(target)
			if err == nil && stat.Size() > 0 {
				backupName := fmt.Sprintf(
					"config-%s-backup.yaml",
					time.Now().Format("20060102_150405"),
				)
				err = os.Rename(target, path.Join(
					config.AppDir(),
					backupName,
				))
				if err != nil {
					l.WithError(err).Errorln("cannot backup existing config")
					os.Exit(1)
				} else {
					l.Infof("existing config file renamed to %s as backup", backupName)
				}
			}

			out, err = os.Create(target)
			if err != nil {
				l.WithError(err).Errorln("cannot create new config")
			}
		} else {
			out = os.Stdout
		}
		file, err := data.ExampleAssets.Open("config.yaml.example")
		if err != nil {
			logrus.WithError(err).Errorln("cannot load sample config resource")
			os.Exit(2)
		}
		n, err := io.Copy(out, file)
		l := logrus.WithField("size", n)
		if err != nil {
			l.WithError(err).Errorln("cannot write config file")
			os.Exit(3)
		}

		l.Infoln("file created successfully")
	},
}
