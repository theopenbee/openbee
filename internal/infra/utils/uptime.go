package utils

import "fmt"

// FormatUptime formats elapsed seconds as "Xh Ym", "Xm Ys", or "Xs".
// Negative values are clamped to "0s".
func FormatUptime(secs int64) string {
	if secs < 0 {
		return "0s"
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%dh %dm", secs/3600, (secs%3600)/60)
}
