package ui

import (
	"bytes"
	"regexp"
	"strings"

	mdterminal "github.com/codewandler/markdown/terminal"
)

var ansiOnlyLineRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TrimOuterRenderedBlankLines removes visually blank leading and trailing lines
// from a rendered string.
func TrimOuterRenderedBlankLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && IsVisuallyBlankRenderedLine(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && IsVisuallyBlankRenderedLine(lines[end-1]) {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// IsVisuallyBlankRenderedLine reports whether a rendered line is blank after
// stripping ANSI SGR codes.
func IsVisuallyBlankRenderedLine(s string) bool {
	s = ansiOnlyLineRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s) == ""
}

// NewMarkdownRendererForWriter returns a Renderer that renders a complete
// markdown string to terminal output using terminal.NewStreamRenderer.
// Kept for use by callers that need a string-in / string-out rendering func.
func NewMarkdownRendererForWriter(_ interface{ Write([]byte) (int, error) }) func(string) string {
	return func(s string) string {
		var buf bytes.Buffer
		sr := mdterminal.NewStreamRenderer(&buf)
		if _, err := sr.Write([]byte(s)); err != nil {
			return strings.TrimRight(s, "\n")
		}
		if err := sr.Flush(); err != nil {
			return strings.TrimRight(s, "\n")
		}
		return TrimOuterRenderedBlankLines(buf.String())
	}
}
