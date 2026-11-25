package watermark

// credit: https://github.com/flopp/go-findfont/blob/master/findfont.go

import (
	"os"
	"path/filepath"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
)

// findFont locates the specified font: the name itself if it is a readable
// file, otherwise a font of that file name in the platform's user and system
// font directories (see lookupFont).
func findFont(fileName string) (filePath string, err error) {
	// check if fileName already points to a readable file
	if _, err := os.Stat(fileName); err == nil {
		return fileName, nil
	}

	return lookupFont(fileName)
}

// isFontFile reports whether a file name looks like a font the renderer may
// be able to load. Collections (.ttc) are excluded: the renderer's parser
// cannot read them.
func isFontFile(fileName string) bool {
	lower := strings.ToLower(fileName)
	return strings.HasSuffix(lower, ".ttf") || strings.HasSuffix(lower, ".otf")
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
