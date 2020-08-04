package install

import (
	"fmt"
	"os/exec"
	"os/user"

	"github.com/sirupsen/logrus"
)

func runCommand(path string, arg ...string) error {
	cmd := exec.Command(path, arg...)
	logrus.Debugf("execute command %s %v", path, arg)
	if out, err := cmd.CombinedOutput(); err != nil {
		logrus.WithField("error", err).Debugf("executing command failed: %s", string(out))
		return err
	}
	return nil
}

func preServiceFileInstall() error {
	name := getAppName()
	user, err := user.Current()
	if err != nil {
		logrus.WithField("error", err).Errorln("cannot get user info")
		return err
	}
	logrus.Debugf("install startup files for user %s(%s)", user.Username, user.Uid)

	logrus.Debugf("check if a plist was installed before")
	err = runCommand(
		"/bin/launchctl",
		"print",
		fmt.Sprintf("gui/%s/%s", user.Uid, name),
	)
	if err == nil {
		logrus.Warnf("service was installed before. bootout it at first")
		_ = runCommand(
			"/bin/launchctl",
			"bootout",
			fmt.Sprintf("gui/%s", user.Uid),
			fmt.Sprintf("%s/Library/LaunchAgents/%s.plist", user.HomeDir, name),
		)
	}
	return nil
}

func postServiceFileInstall() error {
	name := getAppName()
	user, err := user.Current()

	logrus.Debugf("bootstrap the installed plist")
	err = runCommand(
		"/bin/launchctl",
		"bootstrap",
		fmt.Sprintf("gui/%s", user.Uid),
		fmt.Sprintf("%s/Library/LaunchAgents/%s.plist", user.HomeDir, name),
	)
	if err != nil {
		return err
	}
	logrus.Debugf("start the service")
	err = runCommand(
		"/bin/launchctl",
		"kickstart",
		"-k",
		fmt.Sprintf("gui/%s/%s", user.Uid, name),
	)
	return nil
}

func getAppName() string {
	logrus.Debug("using darwin plist filename")
	name := "info.genzj." + defaultProcName
	logrus.Debugf("proc name: %s", name)
	return name
}
