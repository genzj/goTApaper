package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/genzj/goTApaper/api/client"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// noDaemon forces local writes, bypassing any running daemon.
var noDaemon bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and modify the configuration",
	Long: `Inspect and modify the configuration stored in the database.

Settings are typed, and enumerated settings accept only a fixed set of values.
"config list" shows both, so there is no need to consult the documentation to
discover what a setting will accept.`,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every setting with its value, type and accepted values",
	Run: func(cmd *cobra.Command, args []string) {
		settings, err := currentStore().Settings()
		if err != nil {
			logrus.WithError(err).Fatal("cannot read settings")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tVALUE\tSOURCE\tTYPE\tACCEPTS")
		for _, s := range settings {
			def, _ := config.Lookup(s.Key)

			source := "stored"
			if s.IsDefault {
				source = "default"
			}

			accepts := ""
			if len(def.EnumValues) > 0 {
				accepts = strings.Join(def.EnumValues, "|")
			}

			value := s.Value
			if def.Secret && value != "" {
				value = redacted
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Key, value, source, def.Kind, accepts)
		}
		w.Flush()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print one setting's value",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, err := currentStore().Setting(args[0])
		if err != nil {
			logrus.WithError(err).Fatal("cannot read setting")
		}
		fmt.Println(s.Value)
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Change one setting",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key, value := args[0], args[1]

		// Prefer the running daemon, so it applies the change at once instead
		// of waiting out its sleep interval, and so only one process writes
		// the database.
		if client := daemonClient(); client != nil {
			if err := client.SetSetting(key, value); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			logrus.WithField("key", key).WithField("daemon", client.Addr()).Info(
				"setting updated via the running daemon",
			)
			return
		}

		if err := currentStore().SetSetting(key, value); err != nil {
			// The store's error already names the accepted values for an
			// enum, so it is printed as-is rather than summarised.
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.WithField("key", key).Info("setting updated")
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Revert one setting to its default",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if client := daemonClient(); client != nil {
			if err := client.UnsetSetting(args[0]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			logrus.WithField("key", args[0]).Info(
				"setting reverted via the running daemon",
			)
			return
		}

		if err := currentStore().UnsetSetting(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.WithField("key", args[0]).Info("setting reverted to its default")
	},
}

// daemonClient returns a client for a running daemon, or nil when none is
// listening. Writes are routed through it so the daemon reacts immediately and
// the database has a single writer.
func daemonClient() *client.Client {
	if noDaemon {
		return nil
	}
	return client.Discover(config.AppDir())
}

// channel subcommands

var channelCmd = &cobra.Command{
	Use:   "channel",
	Short: "Inspect and modify the configured wallpaper sources",
}

var channelEffective bool

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the configured channels in refresh order",
	Run: func(cmd *cobra.Command, args []string) {
		s := currentStore()
		channels, err := s.Channels()
		if err != nil {
			logrus.WithError(err).Fatal("cannot read channels")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		if channelEffective {
			fmt.Fprintln(w, "NAME\tTYPE\tENABLED\tP\tOPTIONS")
		} else {
			fmt.Fprintln(w, "NAME\tTYPE\tENABLED\tP\tTEMPLATE\tOPTIONS")
		}

		for _, c := range channels {
			options := c.Options
			if channelEffective {
				// Show what the download pipeline will actually see, with the
				// template applied.
				eff, err := s.EffectiveChannel(c.Name)
				if err != nil {
					logrus.WithField("channel", c.Name).WithError(err).Warn(
						"cannot resolve channel",
					)
					continue
				}
				options = eff.Options
				fmt.Fprintf(w, "%s\t%s\t%t\t%.2f\t%s\n",
					c.Name, c.Type, c.Enabled, c.Probability, formatOptions(options))
				continue
			}
			fmt.Fprintf(w, "%s\t%s\t%t\t%.2f\t%s\t%s\n",
				c.Name, c.Type, c.Enabled, c.Probability,
				orDash(c.TemplateName), formatOptions(options))
		}
		w.Flush()
	},
}

var channelEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Include a channel in the refresh rotation",
	Args:  cobra.ExactArgs(1),
	Run:   func(cmd *cobra.Command, args []string) { setChannelEnabled(args[0], true) },
}

var channelDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Exclude a channel from the refresh rotation",
	Long: `Exclude a channel from the refresh rotation.

The channel keeps its configuration and can still be run explicitly with
"goTApaper refresh <name>".`,
	Args: cobra.ExactArgs(1),
	Run:  func(cmd *cobra.Command, args []string) { setChannelEnabled(args[0], false) },
}

func setChannelEnabled(name string, enabled bool) {
	s := currentStore()
	c, err := s.Channel(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	c.Enabled = enabled
	if err := s.UpsertChannel(c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logrus.WithField("channel", name).WithField("enabled", enabled).Info("channel updated")
}

var channelReorderCmd = &cobra.Command{
	Use:   "reorder <name> [name...]",
	Short: "Set the order channels are attempted in",
	Long: `Set the order channels are attempted in.

Every configured channel must be listed, so that no two end up sharing a
position. The last channel always runs, whatever its probability.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := currentStore().ReorderChannels(args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.Info("channel order updated")
	},
}

var channelTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List the available channel types and the options each accepts",
	Run: func(cmd *cobra.Command, args []string) {
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tOPTION\tKIND\tREQUIRED\tACCEPTS\tDESCRIPTION")
		for _, t := range channelTypes() {
			if len(t.Schema.Options) == 0 {
				fmt.Fprintf(w, "%s\t-\t-\t-\t-\ttakes no options\n", t.Name)
				continue
			}
			for _, o := range t.Schema.Options {
				fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\t%s\n",
					t.Name, o.Key, o.Kind, o.Required,
					orDash(strings.Join(o.EnumValues, "|")), o.Description)
			}
		}
		w.Flush()
	},
}

// channel template subcommands

var channelTemplateCmd = &cobra.Command{
	Use:     "channel-template",
	Aliases: []string{"template"},
	Short:   "Manage reusable blocks of channel options",
	Long: `Manage reusable blocks of channel options.

A template holds settings shared by several channels -- typically an API key
and the target dimensions -- so they are configured once rather than repeated.
It replaces what a YAML anchor merged with "<<:" used to do.`,
}

var channelTemplateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the channel templates and what inherits from them",
	Run: func(cmd *cobra.Command, args []string) {
		s := currentStore()
		templates, err := s.ChannelTemplates()
		if err != nil {
			logrus.WithError(err).Fatal("cannot read channel templates")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tUSED BY\tOPTIONS")
		for _, t := range templates {
			users, err := s.ChannelsUsingTemplate(t.Name)
			if err != nil {
				logrus.WithError(err).Warn("cannot list template dependants")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				t.Name, orDash(t.Type), orDash(strings.Join(users, ",")),
				formatOptions(t.Options))
		}
		w.Flush()
	},
}

var channelTemplateSetCmd = &cobra.Command{
	Use:   "set <name> <key=value> [key=value...]",
	Short: "Create or update a channel template",
	Long: `Create or update a channel template.

Only the given keys are changed; the rest of the template is left alone. Every
channel inheriting from it picks up the change immediately.`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		s := currentStore()
		name := args[0]

		t, err := s.ChannelTemplate(name)
		if err != nil {
			// Not found is the create case.
			t = store.ChannelTemplate{Name: name, Options: map[string]any{}}
		}
		if t.Options == nil {
			t.Options = map[string]any{}
		}

		for _, assignment := range args[1:] {
			key, value, ok := strings.Cut(assignment, "=")
			if !ok {
				fmt.Fprintf(os.Stderr, "expected key=value, got %q\n", assignment)
				os.Exit(1)
			}
			t.Options[key] = value
		}

		if err := s.UpsertChannelTemplate(t); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.WithField("template", name).Info("channel template updated")
	},
}

var templateInline bool

var channelTemplateDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a channel template",
	Long: `Delete a channel template.

A template still inherited by a channel is not deleted, because that would
strip those channels of their shared settings. Pass --inline to copy the
settings down into each channel first, which leaves their effective
configuration unchanged.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s := currentStore()
		var err error
		if templateInline {
			err = s.InlineChannelTemplate(args[0])
		} else {
			err = s.DeleteChannelTemplate(args[0])
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.WithField("template", args[0]).Info("channel template deleted")
	},
}

// history subcommand

var configHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show the most recent download for each channel",
	Long: `Show the most recent download for each channel.

This is the deduplication record, one row per channel, used to avoid
downloading the same picture twice. The browsable log of every wallpaper that
has been applied is "goTApaper history".`,
	Run: func(cmd *cobra.Command, args []string) {
		entries, err := currentStore().Histories()
		if err != nil {
			logrus.WithError(err).Fatal("cannot read history")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "CHANNEL\tTITLE\tURL")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.ChannelName, orDash(e.Title), e.URL)
		}
		w.Flush()
	},
}

func init() {
	configCmd.PersistentFlags().BoolVar(
		&noDaemon, "local", false,
		"write directly to the database even if a daemon is running",
	)
	channelListCmd.Flags().BoolVar(
		&channelEffective, "effective", false,
		"show options with the inherited template applied",
	)
	channelTemplateDeleteCmd.Flags().BoolVar(
		&templateInline, "inline", false,
		"copy the template's settings into each channel that inherits it, then delete it",
	)

	channelCmd.AddCommand(
		channelListCmd, channelEnableCmd, channelDisableCmd,
		channelReorderCmd, channelTypesCmd,
	)
	channelTemplateCmd.AddCommand(
		channelTemplateListCmd, channelTemplateSetCmd, channelTemplateDeleteCmd,
	)
	configCmd.AddCommand(
		configListCmd, configGetCmd, configSetCmd, configUnsetCmd,
		channelCmd, channelTemplateCmd, configHistoryCmd,
	)
	RootCmd.AddCommand(configCmd)
}

// formatOptions renders an option map deterministically, so successive runs
// and test assertions see a stable order.
func formatOptions(options map[string]any) string {
	if len(options) == 0 {
		return "-"
	}

	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		value := fmt.Sprintf("%v", options[k])
		if isSecretOption(k) && value != "" {
			value = redacted
		}
		parts = append(parts, k+"="+value)
	}
	return strings.Join(parts, " ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
