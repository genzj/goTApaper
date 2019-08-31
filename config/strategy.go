package config

const (
	// Unknown strategy, normally a placeholder
	Unknown = "unknown"
	// LargestNoLogo downloads the largest possible picture without watermark or embedded channel logo
	LargestNoLogo = "largest-no-logo"
	// Largest downloads the largest resosution offered by a channel, even if with channel's logo or watermark
	Largest = "largest"
	// ByWidth downloads a picture with specified width, must be used with the width parameter
	ByWidth = "by-width"
)
