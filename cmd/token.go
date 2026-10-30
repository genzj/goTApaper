package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/api/wire"
	"github.com/genzj/goTApaper/config"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the token for signing in to the interface or calling the API",
	Long: `Print the token for signing in to the interface or calling the API.

The interface asks for it when opened in a browser that has not signed in
before; opening it from the tray menu signs in automatically. For the API,
send it as "Authorization: Bearer <token>".

The token is created the first time the daemon starts, and is readable only
by the account that runs it.`,
	Run: func(cmd *cobra.Command, args []string) {
		path := wire.TokenPath(config.AppDir())
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr,
				"no token yet: it is created the first time the daemon starts")
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(strings.TrimSpace(string(data)))
	},
}

func init() {
	RootCmd.AddCommand(tokenCmd)
}
