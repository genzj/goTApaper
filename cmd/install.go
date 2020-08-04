// +build !windows

package cmd

import (
	"github.com/genzj/goTApaper/install"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Make goTApaper run at system startup",
	Long:  `Make goTApaper run at system startup`,
	Run: func(cmd *cobra.Command, args []string) {
		installEntry()
	},
}

func init() {
	RootCmd.AddCommand(installCmd)
}

func installEntry() {
	install.InstallStartUp()
}
