package jsonquery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/codewandler/agentsdk/internal/humanize"
	"github.com/codewandler/agentsdk/tool"
)

const (
	maxJSONFileSize      = 10 * 1024 * 1024
	defaultResultLimit   = 100
	maxResultLimit       = 1000
	maxRenderedJSONBytes = 64 * 1024
)

type QueryParams struct {
	Path       string `json:"path" jsonschema:"description=JSON file path to query,required"`
	Expr       string `json:"expr" jsonschema:"description=JSON query expression. Supports jq-like field/index/wildcard paths such as .items[].name or .[0].id,required"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=Maximum number of results to render (default 100 max 1000)"`
}

func Tools() []tool.Tool { return []tool.Tool{Tool()} }

func Tool() tool.Tool {
	return tool.New("json_query",
		"Query a JSON file with a small jq-like path expression. Supports fields, array indexes, and [] wildcards (for example .items[].name).",
		func(ctx tool.Ctx, p QueryParams) (tool.Result, error) {
			if strings.TrimSpace(p.Path) == "" {
				return tool.Error("path cannot be empty"), nil
			}
			if strings.TrimSpace(p.Expr) == "" {
				return tool.Error("expr cannot be empty"), nil
			}
			limit := p.MaxResults
			if limit <= 0 {
				limit = defaultResultLimit
			}
			if limit > maxResultLimit {
				limit = maxResultLimit
			}

			path := resolvePath(p.Path, ctx.WorkDir())
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return tool.Errorf("file not found: %s", path), nil
				}
				return nil, fmt.Errorf("stat %s: %w", path, err)
			}
			if info.IsDir() {
				return tool.Errorf("path is a directory: %s", path), nil
			}
			if info.Size() > maxJSONFileSize {
				return tool.Errorf("file too large: %s (max %s)", humanize.Size(info.Size()), humanize.Size(maxJSONFileSize)), nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			var doc any
			dec := json.NewDecoder(strings.NewReader(string(raw)))
			dec.UseNumber()
			if err := dec.Decode(&doc); err != nil {
				return tool.Errorf("parse JSON %s: %v", path, err), nil
			}
			steps, err := parseExpr(p.Expr)
			if err != nil {
				return tool.Errorf("parse expr %q: %v", p.Expr, err), nil
			}
			matches := applySteps([]any{doc}, steps)
			if len(matches) == 0 {
				return tool.Textf("No matches for %s in %s", p.Expr, path), nil
			}

			rendered := make([]string, 0, min(len(matches), limit))
			bytes := 0
			truncatedBytes := false
			for i, m := range matches {
				if i >= limit {
					break
				}
				text := renderJSON(m)
				if bytes+len(text) > maxRenderedJSONBytes {
					truncatedBytes = true
					break
				}
				bytes += len(text)
				rendered = append(rendered, text)
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "Query: %s\nFile: %s\nMatches: %d", p.Expr, path, len(matches))
			if len(rendered) < len(matches) {
				fmt.Fprintf(&sb, " (showing %d)", len(rendered))
			}
			if truncatedBytes {
				sb.WriteString(" (output truncated by byte limit)")
			}
			sb.WriteString("\n\n")
			if len(rendered) == 1 {
				sb.WriteString(rendered[0])
			} else {
				for i, item := range rendered {
					fmt.Fprintf(&sb, "[%d] %s\n", i, item)
				}
			}
			return tool.Text(strings.TrimSpace(sb.String())), nil
		},
		queryIntent(),
	)
}

type step struct {
	field    string
	index    int
	wildcard bool
	hasIndex bool
}

func parseExpr(expr string) ([]step, error) {
	expr = strings.TrimSpace(expr)
	if expr == "." {
		return nil, nil
	}
	if !strings.HasPrefix(expr, ".") {
		return nil, fmt.Errorf("expression must start with '.'")
	}
	var steps []step
	for i := 1; i < len(expr); {
		switch expr[i] {
		case '.':
			i++
			continue
		case '[':
			st, next, err := parseBracket(expr, i)
			if err != nil {
				return nil, err
			}
			steps = append(steps, st)
			i = next
		default:
			start := i
			for i < len(expr) && expr[i] != '.' && expr[i] != '[' {
				i++
			}
			field := expr[start:i]
			if field == "" {
				return nil, fmt.Errorf("empty field at byte %d", start)
			}
			steps = append(steps, step{field: field})
		}
	}
	return steps, nil
}

func parseBracket(expr string, start int) (step, int, error) {
	end := strings.IndexByte(expr[start:], ']')
	if end < 0 {
		return step{}, 0, fmt.Errorf("missing closing ] at byte %d", start)
	}
	end += start
	inner := strings.TrimSpace(expr[start+1 : end])
	if inner == "" {
		return step{wildcard: true}, end + 1, nil
	}
	if (strings.HasPrefix(inner, "\"") && strings.HasSuffix(inner, "\"")) || (strings.HasPrefix(inner, "'") && strings.HasSuffix(inner, "'")) {
		unquoted, err := strconv.Unquote(strings.ReplaceAll(inner, "'", "\""))
		if err != nil {
			return step{}, 0, fmt.Errorf("invalid quoted field %s", inner)
		}
		return step{field: unquoted}, end + 1, nil
	}
	idx, err := strconv.Atoi(inner)
	if err != nil {
		return step{}, 0, fmt.Errorf("unsupported bracket selector [%s]", inner)
	}
	return step{index: idx, hasIndex: true}, end + 1, nil
}

func applySteps(values []any, steps []step) []any {
	current := values
	for _, st := range steps {
		next := make([]any, 0)
		for _, v := range current {
			switch {
			case st.field != "":
				if obj, ok := v.(map[string]any); ok {
					if val, exists := obj[st.field]; exists {
						next = append(next, val)
					}
				}
			case st.wildcard:
				switch x := v.(type) {
				case []any:
					next = append(next, x...)
				case map[string]any:
					keys := make([]string, 0, len(x))
					for k := range x {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						next = append(next, x[k])
					}
				}
			case st.hasIndex:
				if arr, ok := v.([]any); ok {
					idx := st.index
					if idx < 0 {
						idx = len(arr) + idx
					}
					if idx >= 0 && idx < len(arr) {
						next = append(next, arr[idx])
					}
				}
			}
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return current
}

func renderJSON(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func resolvePath(path, workdir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workdir, path)
}
