package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
	"github.com/sirupsen/logrus"
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

// CopyFile copy content of src to dst
// credit to https://opensource.com/article/18/6/copying-files-go
func CopyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		logrus.Errorf("cannot get stat of the source %s: %s", src, err)
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		err = fmt.Errorf("%s is not a regular file", src)
		logrus.Error(err)
		return 0, err
	}

	source, err := os.Open(src)
	if err != nil {
		logrus.Errorf("cannot open the source %s: %s", src, err)
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		logrus.Errorf("cannot create destination file %s: %s", dst, err)
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	logrus.Debugf("%d bytes copied from %s to %s", nBytes, src, dst)
	if err != nil {
		logrus.Errorf("error in file copying %s", err)
	}
	return nBytes, err
}

type ConfirmRemove func(path string) bool

// RemoveFilesByGlob deletes all files matching a specified glob pattern
func RemoveFilesByGlob(pattern string, confirm ConfirmRemove) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		logrus.Errorf("glob pattern %s failed: %s", pattern, err)
		return err
	}

	for _, path := range matches {
		if confirm(path) {
			logrus.Debugf("remove %s confirmed", path)
			if err = os.Remove(path); err != nil {
				logrus.Errorf("remove %s failed: %s", path, err)
				return err
			}
		} else {
			logrus.Debugf("file %s skipped", path)
		}
	}

	return nil
}
