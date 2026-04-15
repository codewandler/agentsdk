// Package frontmatter provides shared YAML frontmatter parsing for flai.
package frontmatter

import (
	"bufio"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse reads YAML frontmatter from r (delimited by --- lines at the top of the
// input) and returns the parsed map plus the raw body (everything after the
// closing ---).
//
// If no frontmatter is found, (nil, body, nil) is returned.
// The body is the raw content of the input with the frontmatter block removed
// (frontmatter lines and --- delimiters excluded).
func Parse(r io.Reader) (map[string]any, string, error) {
	scanner := bufio.NewScanner(r)

	inFrontmatter := false
	doneFrontmatter := false
	var yamlLines []string
	var bodyLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Once the closing --- has been found, all remaining lines are body.
		if doneFrontmatter {
			bodyLines = append(bodyLines, line)
			continue
		}

		if !inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			// First non-delimiter line before any --- means no frontmatter.
			bodyLines = append(bodyLines, line)
			doneFrontmatter = true
			continue
		}
		// In frontmatter block.
		if strings.TrimSpace(line) == "---" {
			inFrontmatter = false
			doneFrontmatter = true
			continue
		}
		yamlLines = append(yamlLines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, "", err
	}

	// Body is the lines collected before the first ---, joined back together.
	body := strings.TrimLeft(strings.Join(bodyLines, "\n"), "\n")

	if len(yamlLines) == 0 {
		return nil, body, nil
	}

	var result map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &result); err != nil {
		return nil, body, err
	}
	return result, body, nil
}

// StripFrontmatter parses frontmatter from content string.
// Returns the parsed map and the body (content after the frontmatter block).
// Empty or missing frontmatter returns (nil, content).
func StripFrontmatter(content string) (map[string]any, string) {
	m, body, err := Parse(strings.NewReader(content))
	if err != nil {
		return nil, content
	}
	return m, body
}

// Bind populates a struct of type T using YAML struct tags from m.
// Unset fields remain at their zero values. Unknown fields in the map are ignored.
// T must be a struct with yaml tags. Returns an error if m cannot be marshaled
// back through yaml or if T is not a struct.
func Bind[T any](m map[string]any) (T, error) {
	var zero T
	if m == nil {
		return zero, nil
	}

	// Re-marshal through yaml so yaml tags on T resolve correctly.
	data, err := yaml.Marshal(m)
	if err != nil {
		return zero, err
	}
	if err := yaml.Unmarshal(data, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

// BindWithout is like [Bind] but excludes the named fields from the map before
// marshaling. This is useful when a struct field's yaml tag would cause an
// unmarshal error due to type mismatch (e.g. []string but yaml has a plain string).
// The excluded fields can be handled manually from the original map.
func BindWithout[T any](m map[string]any, excludeFields ...string) (T, error) {
	if m == nil {
		var zero T
		return zero, nil
	}

	// Build a copy without the excluded fields.
	filtered := make(map[string]any, len(m)-len(excludeFields))
	excludeSet := make(map[string]bool, len(excludeFields))
	for _, f := range excludeFields {
		excludeSet[f] = true
	}
	for k, v := range m {
		if !excludeSet[k] {
			filtered[k] = v
		}
	}

	return Bind[T](filtered)
}
