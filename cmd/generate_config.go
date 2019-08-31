package cmd

import (
	"io"
	"os"

	"github.com/genzj/goTApaper/data"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(generateConfigCmd)
}

var generateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Write sample configuration file to stdout",
	Long:  `Write sample configuration file to stdout`,
	Run: func(cmd *cobra.Command, args []string) {
		file, err := data.ExampleAssets.Open("config.yaml.example")
		if err != nil {
			logrus.WithError(err).Errorln("cannot load sample config resource")
			os.Exit(-1)
		}
		n, err := io.Copy(os.Stdout, file)
		l := logrus.WithField("size", n)
		if err != nil {
			l.WithError(err).Errorln("cannot write config file")
			os.Exit(-1)
		}

		l.Infoln("file created successfully")
	},
}
