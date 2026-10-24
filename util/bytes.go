package util

import (
	"fmt"
	"strings"
)

// HumanBytes formats a byte count for display.
//
// It lives here rather than beside its first caller because three commands now
// report reclaimed or consumed bytes -- the macOS cache cleanup, the history
// prune and the log file listing -- and two of them are not platform-specific.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0") + " " + suffix
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
