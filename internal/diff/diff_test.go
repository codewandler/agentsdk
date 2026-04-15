package diff_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/core/internal/diff"
)

func TestUnified_NoChange(t *testing.T) {
	content := "line1\nline2\nline3\n"
	result := diff.Unified("file.go", content, content)
	require.Empty(t, result, "no diff when content is identical")
}

func TestUnified_Addition(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline2\nline3\n"
	result := diff.Unified("file.go", old, new)
	require.Contains(t, result, "+line3")
	require.Contains(t, result, "a/file.go")
	require.Contains(t, result, "b/file.go")
}

func TestUnified_Removal(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline3\n"
	result := diff.Unified("file.go", old, new)
	require.Contains(t, result, "-line2")
}

func TestUnified_Replacement(t *testing.T) {
	old := "foo\nbar\n"
	new := "foo\nbaz\n"
	result := diff.Unified("f.go", old, new)
	require.Contains(t, result, "-bar")
	require.Contains(t, result, "+baz")
}

func TestStats_Empty(t *testing.T) {
	added, removed := diff.Stats("")
	require.Equal(t, 0, added)
	require.Equal(t, 0, removed)
}

func TestStats_Addition(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline2\nline3\n"
	unified := diff.Unified("f.go", old, new)
	added, removed := diff.Stats(unified)
	require.Equal(t, 1, added)
	require.Equal(t, 0, removed)
}

func TestStats_Removal(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline3\n"
	unified := diff.Unified("f.go", old, new)
	added, removed := diff.Stats(unified)
	require.Equal(t, 0, added)
	require.Equal(t, 1, removed)
}

func TestStats_Replacement(t *testing.T) {
	old := "foo\nbar\nbaz\n"
	new := "foo\nqux\nquux\nbaz\n"
	unified := diff.Unified("f.go", old, new)
	added, removed := diff.Stats(unified)
	require.Equal(t, 2, added)
	require.Equal(t, 1, removed)
}

func TestStats_IgnoresHeaders(t *testing.T) {
	// Manually constructed diff with --- and +++ headers
	unified := strings.Join([]string{
		"--- a/file.go",
		"+++ b/file.go",
		"@@ -1,2 +1,3 @@",
		" context",
		"-old line",
		"+new line",
		"+extra line",
	}, "\n")
	added, removed := diff.Stats(unified)
	require.Equal(t, 2, added)
	require.Equal(t, 1, removed)
}
