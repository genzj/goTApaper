//go:build !darwin
// +build !darwin

package install

import (
	"github.com/sirupsen/logrus"
)

func preServiceFileInstall() error {
	return nil
}

func postServiceFileInstall(startNow bool) error {
	return nil
}

func preServiceFileUninstall() error {
	return nil
}

func postServiceFileUninstall() error {
	return nil
}

func getAppName() string {
	name := defaultProcName
	logrus.Debugf("proc name: %s", name)
	return name
}
