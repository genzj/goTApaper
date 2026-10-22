package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/config/store/yamlimport"
	"github.com/genzj/goTApaper/data"
)

var exportOutput string

var configExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write the current configuration to YAML",
	Long: `Write the current configuration to YAML.

Shared settings are emitted as YAML anchors, and the channels that inherit them
as "<<:" merge keys, so the result reads like a hand-written file and imports
back to the same state. Settings left at their defaults are omitted.`,
	Run: func(cmd *cobra.Command, args []string) {
		data, err := yamlimport.Export(currentStore())
		if err != nil {
			logrus.WithError(err).Fatal("cannot export the configuration")
		}

		out := os.Stdout
		if exportOutput != "" {
			out, err = os.Create(exportOutput)
			if err != nil {
				logrus.WithError(err).Fatal("cannot create the output file")
			}
			defer out.Close()
		}
		if _, err := out.Write(data); err != nil {
			logrus.WithError(err).Fatal("cannot write the configuration")
		}
	},
}

var configImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Merge a YAML configuration file into the database",
	Long: `Merge a YAML configuration file into the database.

The file is applied in a single transaction, so a failure part-way leaves the
existing configuration untouched. Anchors and merge keys are preserved as
channel templates rather than being flattened.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		content, err := os.ReadFile(args[0])
		if err != nil {
			logrus.WithError(err).Fatal("cannot read the configuration file")
		}
		if err := yamlimport.Import(currentStore(), content); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.WithField("file", args[0]).Info("configuration imported")
	},
}

var configExampleCmd = &cobra.Command{
	Use:   "example",
	Short: "Print the documented example configuration",
	Long: `Print the documented example configuration.

The example is the fullest reference for every available option, since it
carries the explanatory comments that the database cannot. Pipe it through
"config import" to seed a fresh installation from it.`,
	Run: func(cmd *cobra.Command, args []string) {
		file, err := data.ExampleAssets.Open("config.yaml.example")
		if err != nil {
			logrus.WithError(err).Fatal("cannot load the sample configuration resource")
		}
		defer file.Close()

		if _, err := io.Copy(os.Stdout, file); err != nil {
			logrus.WithError(err).Fatal("cannot write the sample configuration")
		}
	},
}

func init() {
	configExportCmd.Flags().StringVarP(
		&exportOutput, "output", "o", "", "write to a file instead of standard output",
	)
	configCmd.AddCommand(configExportCmd, configImportCmd, configExampleCmd)
}
