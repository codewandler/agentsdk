package ui

import "fmt"

// ToolSummarizer extracts a short display string from tool arguments.
// Return "" to fall back to compact (name-only) display.
type ToolSummarizer func(args map[string]any) string

var toolSummarizers = map[string]ToolSummarizer{
	"file_read":   summarizePath,
	"file_write":  summarizePath,
	"file_edit":   summarizePath,
	"file_delete": summarizePath,
	"file_stat":   summarizePath,
	"grep":        func(a map[string]any) string { return stringKey(a, "pattern") },
	"bash":        summarizeBash,
	"git_commit":  func(a map[string]any) string { return stringKey(a, "message") },
	"web_fetch":   func(a map[string]any) string { return stringKey(a, "url") },
	"web_search":  func(a map[string]any) string { return stringKey(a, "query") },
	"glob":        func(a map[string]any) string { return stringKey(a, "pattern") },
	"dir_tree":    summarizePath,
	"dir_list":    summarizePath,
}

func summarizePath(args map[string]any) string {
	return stringOrArrayKey(args, "path")
}

func summarizeBash(args map[string]any) string {
	s := stringOrArrayKey(args, "cmd")
	// Truncate long commands
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

func stringOrArrayKey(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if arr, ok := v.([]any); ok && len(arr) > 0 {
		if first, ok := arr[0].(string); ok {
			if len(arr) == 1 {
				return first
			}
			return fmt.Sprintf("%s (+%d more)", first, len(arr)-1)
		}
	}
	return ""
}

func stringKey(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
