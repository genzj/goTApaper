package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the main version number that is being run at the moment.
var Version string

// GitCommit is the commit that was compiled. This will be filled in by the compiler.
var GitCommit string

// VersionPrerelease marker for the version. If this is "" (empty string)
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
