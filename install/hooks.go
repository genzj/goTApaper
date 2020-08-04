// +build !darwin

package install

import (
	"github.com/sirupsen/logrus"
)

func preServiceFileInstall() error {
	return nil
}

func postServiceFileInstall() error {
	return nil
}

func getAppName() string {
	name := defaultProcName
	logrus.Debugf("proc name: %s", name)
	return name
}
