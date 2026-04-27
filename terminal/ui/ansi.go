// Package ui provides terminal rendering helpers for agentsdk applications.
package ui

const (
	Reset        = "\033[0m"
	Bold         = "\033[1m"
	Dim          = "\033[2m"
	BrightRed    = "\033[91m"
	BrightGreen  = "\033[92m"
	BrightYellow = "\033[93m"
	BrightCyan   = "\033[96m"
	CodePink     = "\033[38;2;203;166;247m"
	Italic       = "\033[3m"
)

// Truncate returns s shortened to max bytes with an ASCII ellipsis marker.
func Truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
