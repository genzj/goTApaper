package actor

import (
	"os/exec"

	"github.com/genzj/goTApaper/util"
	"github.com/sirupsen/logrus"
)

// Setter set OS desktop to use downloaded wallpaper
type Setter interface {
	Set(filename string) error
}

// Setters singleton for convenience
var Setters = util.RegistryMap{}

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
