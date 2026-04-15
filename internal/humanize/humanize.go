// Package humanize provides human-readable formatting utilities.
package humanize

import "fmt"

// Size formats a byte count as a human-readable string.
// Examples: 512B, 1.5KB, 2.3MB, 4.0GB.
func Size(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1fGB", float64(n)/(1024*1024*1024))
	}
}
