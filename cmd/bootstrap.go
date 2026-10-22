package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/config/store/sqlite"
	"github.com/genzj/goTApaper/config/store/yamlimport"
	"github.com/genzj/goTApaper/history"
)

// provider is the process-wide configuration provider, created by loadConfig.
// Commands reach the store through it.
var provider *store.Provider

// currentStore returns the open store, or exits if configuration was never
// loaded. Commands run under cobra.OnInitialize, so this is a programming
// error rather than a user-facing one.
func currentStore() store.Store {
	if provider == nil {
		logrus.Fatal("configuration accessed before it was loaded")
	}
	return provider.Store()
}

// loadConfig opens the configuration database, importing a legacy config.yaml
// on first run, and installs the resulting provider as the process-wide
// configuration source.
func loadConfig() {
	config.SetAppName(AppName)
	if appDir != "" {
		config.SetAppDir(appDir)
	}
	config.EnsureAppDir()

	dbPath := config.DBFileName()
	fresh := !fileExists(dbPath)

	s, err := sqlite.Open(dbPath)
	if err != nil {
		if errors.Is(err, store.ErrSchemaTooNew) {
			// Worth its own message: the fix is to upgrade, not to debug.
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logrus.WithField("database", dbPath).WithError(err).Fatal(
			"cannot open the configuration database",
		)
	}

	provider = store.NewProvider(s)
	config.SetCurrent(provider)
	history.SetDefault(store.NewHistoryManager(s))

	// Only a brand new database triggers an import. Once the YAML has been
	// consumed and renamed, the database is the single source of truth, and a
	// stray config.yaml must not quietly override it.
	if fresh {
		if err := yamlimport.ImportIfPresent(s, config.LegacyConfigFileName()); err != nil {
			logrus.WithError(err).Fatal("cannot import the existing configuration file")
		}
	}

	applyFlagOverrides()

	// Applied twice overall: once with defaults so early messages are routed,
	// and again here now that the stored debug flag and any --debug override
	// are known.
	config.InitLogger()
	config.Observe(config.KeyLogDebug, func(string, any, any) {
		config.InitLogger()
	})
}

// applyFlagOverrides pushes command-line flags into the provider's override
// layer. They win for this run without being written to the database, so
// `goTApaper --debug refresh` does not leave debug logging on afterwards.
func applyFlagOverrides() {
	root := RootCmd.PersistentFlags()
	for key, flagName := range map[string]string{
		config.KeyLogDebug:          "debug",
		config.KeyLogDebugRendering: "debug-rendering",
		config.KeyLanguage:          "lang",
		config.KeyHistoryFile:       "history-file",
	} {
		applyFlagOverride(key, root.Lookup(flagName))
	}

	// Subcommand flags that also shadow a setting.
	applyFlagOverride(config.KeyWallpaperSetter, refreshCmd.PersistentFlags().Lookup("setter"))
	applyFlagOverride(config.KeyDaemonInterval, daemonCmd.PersistentFlags().Lookup("interval"))
}

// applyFlagOverride records one flag as an override, but only if the user
// actually passed it: a flag left at its default must not shadow a stored
// value.
func applyFlagOverride(key string, f *pflag.Flag) {
	if f == nil || !f.Changed {
		return
	}
	if err := provider.Override(key, f.Value.String()); err != nil {
		logrus.WithField("flag", f.Name).WithError(err).Fatal("invalid command line value")
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}
