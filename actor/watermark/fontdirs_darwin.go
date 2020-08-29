package watermark

func getFontDirectories() (paths []string) {
	return []string{
		expandUser("~/Library/Fonts/"),
		"/Library/Fonts/",
		"/System/Library/Fonts/",
	}
}
