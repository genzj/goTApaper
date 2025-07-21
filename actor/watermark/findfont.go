package watermark

// credit: https://github.com/flopp/go-findfont/blob/master/findfont.go

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
)

// findFind tries to locate the specified font file in the directory as
// well as in platform specific user and system font directories; if there is
// no exact match, tries substring matching.
func findFont(fileName string) (filePath string, err error) {
	// check if fileName already points to a readable file
	if _, err := os.Stat(fileName); err == nil {
		return fileName, nil
	}

	// search in user and system directories
	return find(filepath.Base(fileName))
}

func isFontFile(fileName string) bool {
	lower := strings.ToLower(fileName)
	return strings.HasSuffix(lower, ".ttf") || strings.HasSuffix(lower, ".ttc")
}

func stripExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

func expandUser(path string) (expandedPath string) {
	expandedPath, err := homedir.Expand(path)
	if err == nil {
		return expandedPath
	}
	logrus.WithError(err).Warnf(
		"cannot expand user path %s, will use it as-is", path,
	)
	return path
}

func find(needle string) (filePath string, err error) {
	lowerNeedle := strings.ToLower(needle)
	lowerNeedleBase := stripExtension(lowerNeedle)

	match := ""
	partial := ""
	partialScore := -1

	walkF := func(path string, info os.FileInfo, err error) error {
		// we have already found a match -> nothing to do
		if match != "" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err != nil {
			return nil
		}

		lowerPath := strings.ToLower(info.Name())

		if info.IsDir() == false && isFontFile(lowerPath) {
			lowerBase := stripExtension(lowerPath)
			if lowerPath == lowerNeedle {
				// exact match
				match = path
			} else if strings.Contains(lowerBase, lowerNeedleBase) {
				// partial match
				score := len(lowerBase) - len(lowerNeedle)
				if partialScore < 0 || score < partialScore {
					partialScore = score
					partial = path
				}
			}
		}
		return nil
	}

	for _, dir := range getFontDirectories() {
		_ = filepath.Walk(dir, walkF)
		if match != "" {
			return match, nil
		}
	}

	if partial != "" {
		return partial, nil
	}

	return "", fmt.Errorf("cannot find font '%s' in user or system directories", needle)
}
