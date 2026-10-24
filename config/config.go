package config

import (
	"fmt"
	"os"
	"path"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/logging"
)

// InitLogger configures the logger from the current settings. It is exported
// so that callers can re-apply it once configuration is loaded, and again
// whenever the logging settings change.
func InitLogger() {
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	if GetBool(KeyLogDebug) {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugln("Debug log enabled")
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	installLogSinks()
}

// installLogSinks attaches the in-memory ring and the rotating file.
//
// It is skipped until the application directory is known, because resolving
// the log path calls AppDir, which panics before SetAppName has run. A unit
// test that only reads a setting therefore gets stderr logging and nothing
// else, which is what it wants.
func installLogSinks() {
	if appName == "" {
		return
	}

	if err := logging.Install(GetInt(KeyLogRingSize), logging.RotateOptions{
		Path:       GetLogFileName(),
		MaxSizeMB:  GetInt(KeyLogMaxSizeMB),
		MaxBackups: GetInt(KeyLogMaxBackups),
	}); err != nil {
		// Logged, not fatal: goTApaper's job is to set wallpapers, and it
		// should keep doing that when it cannot open a log file. stderr and
		// the in-memory ring still work.
		logrus.WithError(err).Error("cannot open the log file, continuing without it")
	}
}

var appName string

// SetAppName for app dir path finding
func SetAppName(name string) {
	appName = name
}

func getAppName() string {
	if appName == "" {
		panic("calling configuration related methods prior to LoadConfig")
	}
	return appName
}

var appDirOverride string

// SetAppDir overrides the application directory, normally from the --app-dir
// flag. An empty value restores the default.
func SetAppDir(dir string) {
	appDirOverride = dir
}

// appDirEnvVar names the environment variable that overrides the app dir.
func appDirEnvVar() string {
	return strings.ToUpper(getAppName()) + "_APP_DIR"
}

// AppDir returns a path to the user config directory, resolved from the
// --app-dir flag, then the <APPNAME>_APP_DIR environment variable, then the
// platform's conventional location.
//
// The override exists so tests and throwaway profiles never have to touch the
// real user directory.
func AppDir() string {
	if appDirOverride != "" {
		return mustExpandAppDir(appDirOverride)
	}
	if env := os.Getenv(appDirEnvVar()); env != "" {
		return mustExpandAppDir(env)
	}

	// An existing legacy directory wins over the platform default, so an
	// upgrade does not strand a user's configuration somewhere the new build
	// no longer looks.
	legacy := mustExpandAppDir(legacyAppDir())
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		return legacy
	}

	if dir := platformAppDir(); dir != "" {
		return dir
	}
	return legacy
}

// legacyAppDir is the dotfile directory used before per-platform locations
// were honoured. Still read when it exists.
func legacyAppDir() string {
	return "~/." + getAppName()
}

func mustExpandAppDir(dir string) string {
	expanded, err := homedir.Expand(dir)
	if err != nil {
		fmt.Println("cannot retrive home dir path")
		fmt.Println(err)
		os.Exit(-1)
	}
	return expanded
}

// EnsureAppDir creates app dir if it doesn't exist
func EnsureAppDir() {
	if err := os.MkdirAll(AppDir(), 0755); err != nil {
		fmt.Println("cannot create app dir " + AppDir())
		fmt.Println(err)
		os.Exit(-1)
	}
}

// DBFileName returns the path to the SQLite configuration database.
func DBFileName() string {
	return path.Join(AppDir(), DefaultDBFileName)
}

// LegacyConfigFileName returns the path of the YAML configuration file that
// the database replaced. It is read once, on first run, and then renamed.
func LegacyConfigFileName() string {
	return path.Join(AppDir(), "config.yaml")
}
