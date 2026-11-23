package setter

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
)

// Setter set OS desktop to use downloaded wallpaper
type Setter interface {
	Set(filename string) error
}

// setterMap is the setter registry. It wraps the shared registry so that
// registering a setter also declares it as a valid value for the `setter`
// setting.
type setterMap struct {
	util.RegistryMap
}

// Register adds a setter and records its name in the configuration schema.
//
// The schema is told one name at a time, as each setter registers itself,
// rather than being handed the registry to read later. Go runs a package's
// init functions in filename order, so a setter in a file sorting after this
// one (winsetter.go, xfcesetter.go) registers after this file's init: anything
// that snapshots the registry up front would miss them.
func (m setterMap) Register(name string, s interface{}) {
	m.RegistryMap.Register(name, s)
	config.RegisterEnumValue(config.KeyWallpaperSetter, name)
}

// Setters singleton for convenience
var Setters = setterMap{RegistryMap: util.RegistryMap{}}

func setWithCommand(path string, arg ...string) error {
	cmd := exec.Command(path, arg...)
	logrus.Debugf("executing setter with command %s %v", path, arg)
	if err := cmd.Start(); err != nil {
		logrus.Errorf("executing setter failed: %s", err)
		return err
	}
	go func() {
		cmd.Wait()
	}()
	return nil
}

func runCommand(path string, arg ...string) (string, error) {
	cmd := exec.Command(path, arg...)
	logrus.Debugf("executing command %s %v", path, arg)

	// Output() internally calls Start() and Wait() and captures stdout
	out, err := cmd.Output()
	logrus.Debugf("command output: %s", out)
	if err != nil {
		logrus.Errorf("executing command failed: %s", err)
		return string(out), err
	}

	return string(out), nil
}

// samePathOrFile reports whether a and b name the same file: the same cleaned
// path, or two paths (symlinks, differently spelled directories) that stat to
// the same file. Either being empty, or not statable, counts as different.
//
// Setters use it to tell whether a requested wallpaper is the one the desktop
// already shows, since shells ignore a set to the path they are displaying.
func samePathOrFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	infoA, errA := os.Stat(a)
	if errA != nil {
		return false
	}
	infoB, errB := os.Stat(b)
	if errB != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}
