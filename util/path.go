package util

import (
	"github.com/Sirupsen/logrus"
	"github.com/kardianos/osext"
)

// ExecutableFolder returns path to the folder containing currently running
// executable file
func ExecutableFolder() string {
	folderPath, err := osext.ExecutableFolder()
	if err != nil {
		logrus.Fatal(err)
	}
	return folderPath
}
