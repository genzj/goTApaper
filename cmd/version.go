package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// The main version number that is being run at the moment.
const Version = "0.1.0"

// The git commit that was compiled. This will be filled in by the compiler.
var GitCommit string

// A pre-release marker for the version. If this is "" (empty string)
// then it means that it is a final release. Otherwise, this is a pre-release
// such as "dev" (in development)
var VersionPrerelease = "dev"

func init() {
	RootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Show version information`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(
			"%s - %s %s(%s)\n",
			AppName,
			Version,
			VersionPrerelease,
			GitCommit,
		)
	},
}
