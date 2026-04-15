// Package diff provides utilities for generating and parsing unified diffs.
package diff

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// Unified generates a unified diff between old and new content.
// path is used in the diff header (a/path, b/path).
// Returns an empty string when old == new.
func Unified(path, old, new string) string {
	d := difflib.UnifiedDiff{
		A:        difflib.SplitLines(old),
		B:        difflib.SplitLines(new),
		FromFile: "a/" + path,
		ToFile:   "b/" + path,
		Context:  3,
	}
	text, _ := difflib.GetUnifiedDiffString(d)
	return text
}

// Stats counts the number of added and removed lines in a unified diff string.
// It inspects lines starting with '+' or '-' but ignores the file headers (--- / +++).
func Stats(unified string) (added, removed int) {
	for _, line := range strings.Split(unified, "\n") {
		if len(line) == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			// header lines — skip
		case line[0] == '+':
			added++
		case line[0] == '-':
			removed++
		}
	}
	return added, removed
}
