// Package toolmgmt provides tools for dynamically activating and deactivating tools.
package toolmgmt

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codewandler/core/tool"
	rttool "github.com/codewandler/flai/runtime/tool"
)

// KeyActivationState is the Extra() key under which tools_* look up the ActivationState.
const KeyActivationState = "flai.activation_state"

// ToolListParams has no fields — tools_list always returns everything.
type ToolListParams struct{}

// ToolActivateParams for tools_activate.
type ToolActivateParams struct {
	Tools tool.StringSliceParam `json:"tools" jsonschema:"description=Tool names or patterns to activate. Can be a single string or array of strings (e.g. \"file_*\" or [\"file_*\"\\, \"bash\"]).,required"`
}

// ToolDeactivateParams for tools_deactivate.
type ToolDeactivateParams struct {
	Tools tool.StringSliceParam `json:"tools" jsonschema:"description=Tool names or patterns to deactivate. Can be a single string or array of strings (e.g. \"file_*\").,required"`
}

// activation extracts the ActivationState from ctx.Extra(), returning a clear
// error when it is missing (mis-configured injection).
func activation(ctx tool.Ctx) (*rttool.ActivationState, error) {
	v, ok := ctx.Extra()[KeyActivationState]
	if !ok {
		return nil, fmt.Errorf("tools_* tools require flai.activation_state in Extra(); check agent wiring")
	}
	state, ok := v.(*rttool.ActivationState)
	if !ok {
		return nil, fmt.Errorf("flai.activation_state has unexpected type %T", v)
	}
	return state, nil
}

// matchesPattern reports whether name matches a glob pattern.
func matchesPattern(name, pattern string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// Tools returns the three tool-management tools. They read the ActivationState
// from ctx.Extra() on every call — no constructor injection needed.
func Tools() []tool.Tool {
	return []tool.Tool{
		toolsList(),
		toolsActivate(),
		toolsDeactivate(),
	}
}

// ── tools_list ────────────────────────────────────────────────────────────────

type listEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

type listResult struct {
	Active   []listEntry `json:"active"`
	Inactive []listEntry `json:"inactive"`
}

func (r listResult) IsError() bool { return false }
func (r listResult) String() string {
	var sb strings.Builder
	if len(r.Active) > 0 {
		sb.WriteString("### Active\n")
		for _, e := range r.Active {
			fmt.Fprintf(&sb, "- **%s**: %s\n", e.Name, e.Description)
		}
	} else {
		sb.WriteString("### Active\n(none)\n")
	}
	sb.WriteString("\n")
	if len(r.Inactive) > 0 {
		sb.WriteString("### Inactive\n")
		for _, e := range r.Inactive {
			fmt.Fprintf(&sb, "- **%s**: %s\n", e.Name, e.Description)
		}
	} else {
		sb.WriteString("### Inactive\n(none)\n")
	}
	return strings.TrimSpace(sb.String())
}
func (r listResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type     string      `json:"type"`
		Active   []listEntry `json:"active"`
		Inactive []listEntry `json:"inactive"`
	}
	return json.Marshal(wire{Type: "tools_list", Active: r.Active, Inactive: r.Inactive})
}

func toolsList() tool.Tool {
	return tool.New("tools_list",
		"List all available tools and their activation status. Shows which tools are currently active and which can be activated.",
		func(ctx tool.Ctx, _ ToolListParams) (tool.Result, error) {
			state, err := activation(ctx)
			if err != nil {
				return tool.Error(err.Error()), nil
			}
			all := state.AllTools()
			activeSet := make(map[string]bool)
			for _, t := range state.ActiveTools() {
				activeSet[t.Name()] = true
			}
			var active, inactive []listEntry
			for _, t := range all {
				entry := listEntry{Name: t.Name(), Description: t.Description()}
				if activeSet[t.Name()] {
					entry.Active = true
					active = append(active, entry)
				} else {
					inactive = append(inactive, entry)
				}
			}
			return listResult{Active: active, Inactive: inactive}, nil
		},
	)
}

// ── tools_activate ────────────────────────────────────────────────────────────

type activateResult struct {
	Activated     []string `json:"activated"`
	AlreadyActive []string `json:"already_active,omitempty"`
	NotFound      []string `json:"not_found,omitempty"`
}

func (r activateResult) IsError() bool { return false }
func (r activateResult) String() string {
	var sb strings.Builder
	if len(r.Activated) > 0 {
		fmt.Fprintf(&sb, "Activated: %s\n", strings.Join(r.Activated, ", "))
	}
	if len(r.AlreadyActive) > 0 {
		fmt.Fprintf(&sb, "Already active: %s\n", strings.Join(r.AlreadyActive, ", "))
	}
	if len(r.NotFound) > 0 {
		fmt.Fprintf(&sb, "Not found: %s\n", strings.Join(r.NotFound, ", "))
	}
	return strings.TrimSpace(sb.String())
}
func (r activateResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type          string   `json:"type"`
		Activated     []string `json:"activated"`
		AlreadyActive []string `json:"already_active,omitempty"`
		NotFound      []string `json:"not_found,omitempty"`
	}
	return json.Marshal(wire{Type: "tools_activate", Activated: r.Activated,
		AlreadyActive: r.AlreadyActive, NotFound: r.NotFound})
}

func toolsActivate() tool.Tool {
	return tool.New("tools_activate",
		`Activate tools by name or pattern. Use patterns like "file_*" to activate multiple tools at once. Activated tools will be available until the session ends or deactivated.`,
		func(ctx tool.Ctx, p ToolActivateParams) (tool.Result, error) {
			if len(p.Tools) == 0 {
				return tool.Error("at least one tool pattern is required"), nil
			}
			state, err := activation(ctx)
			if err != nil {
				return tool.Error(err.Error()), nil
			}

			all := state.AllTools()
			activeSet := make(map[string]bool)
			for _, t := range state.ActiveTools() {
				activeSet[t.Name()] = true
			}

			var alreadyActive, notFound []string
			for _, pattern := range p.Tools {
				matched := false
				for _, t := range all {
					if matchesPattern(t.Name(), pattern) {
						matched = true
						if activeSet[t.Name()] {
							alreadyActive = append(alreadyActive, t.Name())
						}
					}
				}
				if !matched {
					notFound = append(notFound, pattern)
				}
			}

			activated := state.Activate(p.Tools...)

			// Deduplicate alreadyActive
			seen := make(map[string]bool)
			var unique []string
			for _, n := range alreadyActive {
				if !seen[n] {
					seen[n] = true
					unique = append(unique, n)
				}
			}

			if len(activated) == 0 && len(unique) == 0 {
				return tool.Textf("No tools matched patterns: %v\nUse tools_list to see available tools.", p.Tools), nil
			}
			return activateResult{Activated: activated, AlreadyActive: unique, NotFound: notFound}, nil
		},
	)
}

// ── tools_deactivate ──────────────────────────────────────────────────────────

type deactivateResult struct {
	Deactivated []string `json:"deactivated"`
}

func (r deactivateResult) IsError() bool { return false }
func (r deactivateResult) String() string {
	return "Deactivated: " + strings.Join(r.Deactivated, ", ")
}
func (r deactivateResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type        string   `json:"type"`
		Deactivated []string `json:"deactivated"`
	}
	return json.Marshal(wire{Type: "tools_deactivate", Deactivated: r.Deactivated})
}

func toolsDeactivate() tool.Tool {
	return tool.New("tools_deactivate",
		`Deactivate tools by name or pattern. Use patterns like "file_*" to deactivate multiple tools at once. Note: Some essential tools (like turn_done, tools_activate) cannot be deactivated.`,
		func(ctx tool.Ctx, p ToolDeactivateParams) (tool.Result, error) {
			if len(p.Tools) == 0 {
				return tool.Error("at least one tool pattern is required"), nil
			}
			state, err := activation(ctx)
			if err != nil {
				return tool.Error(err.Error()), nil
			}
			deactivated := state.Deactivate(p.Tools...)
			if len(deactivated) == 0 {
				return tool.Textf("No tools deactivated for patterns: %v\n(Tools may not exist, already be inactive, or be protected)", p.Tools), nil
			}
			return deactivateResult{Deactivated: deactivated}, nil
		},
	)
}
