package watermark

import (
	"os"
	"path/filepath"
)

func getFontDirectories() (paths []string) {
	return []string{
		filepath.Join(os.Getenv("windir"), "Fonts"),
		filepath.Join(os.Getenv("localappdata"), "Microsoft", "Windows", "Fonts"),
	}
}
