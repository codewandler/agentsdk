package appconfig

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Loader loads and merges appconfig documents from multiple paths with
// cycle detection and include expansion.
type Loader struct {
	visited map[string]bool
	result  LoadResult
}

// NewLoader creates a Loader.
func NewLoader() *Loader {
	return &Loader{visited: make(map[string]bool)}
}

// Load processes one or more file paths or glob patterns. Each path is
// expanded, deduplicated, and loaded. Include directives in kind=config
// documents are recursively expanded.
func (l *Loader) Load(paths ...string) (LoadResult, error) {
	// Expand all paths.
	var resolved []string
	for _, pattern := range paths {
		expanded := expandVars(pattern, ".")
		if isGlob(expanded) {
			matches, err := filepath.Glob(expanded)
			if err != nil {
				return LoadResult{}, fmt.Errorf("appconfig: glob %q: %w", pattern, err)
			}
			sort.Strings(matches)
			resolved = append(resolved, matches...)
		} else {
			resolved = append(resolved, expanded)
		}
	}
	// Dedup.
	resolved = dedup(resolved)

	// Load each file.
	for _, path := range resolved {
		if err := l.loadPath(path); err != nil {
			return LoadResult{}, err
		}
	}

	// Set entry path from first loaded file if not set.
	if l.result.EntryPath == "" && len(resolved) > 0 {
		abs, _ := filepath.Abs(resolved[0])
		l.result.EntryPath = abs
	}

	return l.result, nil
}

// loadPath loads a single file with cycle detection, then processes
// any include directives found in kind=config documents.
func (l *Loader) loadPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("appconfig: resolve %s: %w", path, err)
	}
	if l.visited[abs] {
		return nil // cycle: already loaded
	}
	l.visited[abs] = true

	if err := loadFileInto(&l.result, abs); err != nil {
		return err
	}

	// Collect and process includes from any config documents just loaded.
	// We snapshot the includes and clear them to avoid re-processing.
	includes := l.result.Config.Include
	l.result.Config.Include = nil

	baseDir := filepath.Dir(abs)
	for _, pattern := range includes {
		expanded := expandVars(pattern, baseDir)
		if !isGlob(expanded) {
			if err := l.loadPath(expanded); err != nil {
				return fmt.Errorf("appconfig: include %s: %w", pattern, err)
			}
			continue
		}
		matches, err := filepath.Glob(expanded)
		if err != nil {
			return fmt.Errorf("appconfig: glob %q (expanded: %q): %w", pattern, expanded, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if err := l.loadPath(match); err != nil {
				return fmt.Errorf("appconfig: include %s: %w", match, err)
			}
		}
	}

	return nil
}

func isGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func dedup(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, p)
	}
	return out
}
