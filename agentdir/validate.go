package agentdir

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/resource"
)

// ValidationResult is the structured output of Validate.
type ValidationResult struct {
	Dir      string             `json:"dir"`
	Manifest ManifestValidation `json:"manifest"`
	Agents   []AgentValidation  `json:"agents"`
	Skills   SkillsValidation   `json:"skills"`

	Workflows          []string `json:"workflows"`
	Commands           []string `json:"commands"`
	StructuredCommands []string `json:"structuredCommands"`
	Actions            []string `json:"actions"`
	Triggers           []string `json:"triggers"`

	Checks []Check `json:"checks"`
}

// OK returns true when no check has status "error".
func (r ValidationResult) OK() bool {
	for _, c := range r.Checks {
		if c.Status == StatusError {
			return false
		}
	}
	return true
}

// ManifestValidation reports manifest-level findings.
type ManifestValidation struct {
	Found               bool     `json:"found"`
	Path                string   `json:"path,omitempty"`
	DefaultAgent        string   `json:"defaultAgent,omitempty"`
	Sources             []string `json:"sources,omitempty"`
	GlobalUserResources *bool    `json:"globalUserResources,omitempty"`
}

// AgentValidation reports per-agent findings.
type AgentValidation struct {
	Name           string   `json:"name"`
	HasFrontmatter bool     `json:"hasFrontmatter"`
	Tools          []string `json:"tools,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	HasSystem      bool     `json:"hasSystem"`
	MaxSteps       int      `json:"maxSteps,omitempty"`
}

// SkillsValidation reports skill discovery findings.
type SkillsValidation struct {
	Local            []string            `json:"local,omitempty"`
	GlobalAvailable  []string            `json:"globalAvailable,omitempty"`
	GlobalIncluded   bool                `json:"globalIncluded"`
	AgentSkillRefs   map[string][]string `json:"agentSkillRefs,omitempty"`
	Unresolvable     []string            `json:"unresolvable,omitempty"`
}

// Check is a single validation finding.
type Check struct {
	Category string `json:"category"`
	Subject  string `json:"subject,omitempty"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

const (
	StatusPassed  = "passed"
	StatusWarning = "warning"
	StatusError   = "error"
)

// ValidateOptions configures Validate behavior.
type ValidateOptions struct {
	// HomeDir overrides the user home directory for global skill scanning.
	// If empty, os.UserHomeDir is used.
	HomeDir string
}

// Validate performs structural validation of an agentsdk app directory.
// It resolves the manifest, agents, skills, workflows, commands, and actions,
// then runs checks that report errors, warnings, and passed validations.
func Validate(dir string, opts ValidateOptions) (ValidationResult, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ValidationResult{}, err
	}

	result := ValidationResult{Dir: dir}

	// ── Manifest ──────────────────────────────────────────────────────────

	manifestPath, manifest, hasManifest, err := readManifest(dir)
	if err != nil {
		return ValidationResult{}, err
	}

	result.Manifest.Found = hasManifest
	if hasManifest {
		result.Manifest.Path = manifestPath
		result.Manifest.DefaultAgent = manifest.DefaultAgent
		result.Manifest.Sources = manifest.Sources
		result.Manifest.GlobalUserResources = manifest.Discovery.IncludeGlobalUserResources

		result.addCheck("manifest", "", StatusPassed, fmt.Sprintf("manifest found at %s", filepath.Base(manifestPath)))

		if len(manifest.Sources) == 0 {
			result.addCheck("manifest", "", StatusError, "manifest has no \"sources\" field — discovery will fall back to scanning .agents/.claude directories")
		} else {
			result.addCheck("manifest", "", StatusPassed, fmt.Sprintf("manifest declares %d source(s)", len(manifest.Sources)))
		}

		if manifest.DefaultAgent == "" {
			result.addCheck("manifest", "", StatusWarning, "manifest has no \"default_agent\" — runtime will pick the first discovered agent")
		} else {
			result.addCheck("manifest", "", StatusPassed, fmt.Sprintf("default_agent: %s", manifest.DefaultAgent))
		}
	} else {
		result.addCheck("manifest", "", StatusWarning, "no manifest found (agentsdk.app.json or app.manifest.json) — using fallback directory scan")
	}

	// ── Resolve resources (local-only to avoid side effects) ──────────────

	resolved, resolveErr := ResolveDirWithOptions(dir, ResolveOptions{
		Policy:  resource.DiscoveryPolicy{},
		HomeDir: opts.HomeDir,
	})
	if resolveErr != nil {
		result.addCheck("discovery", "", StatusError, fmt.Sprintf("resource resolution failed: %v", resolveErr))
		return result, nil // return partial result, not an error
	}

	// Forward existing discovery diagnostics.
	for _, diag := range resolved.Bundle.Diagnostics {
		status := StatusWarning
		if diag.Severity == resource.SeverityError {
			status = StatusError
		}
		result.addCheck("discovery", "", status, diag.Message)
	}

	// ── Agents ────────────────────────────────────────────────────────────

	agentNames := map[string]bool{}
	for _, spec := range resolved.Bundle.AgentSpecs {
		agentNames[spec.Name] = true
		av := AgentValidation{
			Name:      spec.Name,
			Tools:     spec.Tools,
			Skills:    spec.Skills,
			Commands:  spec.Commands,
			HasSystem: strings.TrimSpace(spec.System) != "",
			MaxSteps:  spec.MaxSteps,
		}

		// Check frontmatter presence: if the spec has tools, skills, capabilities,
		// or a description, it was parsed from frontmatter. A spec with only a name
		// (derived from filename) and system prompt but nothing else likely has no
		// frontmatter.
		av.HasFrontmatter = spec.Description != "" || len(spec.Tools) > 0 ||
			len(spec.Skills) > 0 || len(spec.Capabilities) > 0 ||
			len(spec.Commands) > 0 || spec.MaxSteps > 0

		for _, cap := range spec.Capabilities {
			av.Capabilities = append(av.Capabilities, cap.CapabilityName)
		}

		result.Agents = append(result.Agents, av)

		if !av.HasFrontmatter {
			result.addCheck("agent", spec.Name, StatusError, fmt.Sprintf("agent %q has no YAML frontmatter — no tools, skills, or capabilities configured", spec.Name))
		} else {
			result.addCheck("agent", spec.Name, StatusPassed, fmt.Sprintf("agent %q has frontmatter", spec.Name))
		}

		if !av.HasSystem {
			result.addCheck("agent", spec.Name, StatusWarning, fmt.Sprintf("agent %q has no system prompt content", spec.Name))
		}

		if len(spec.Tools) == 0 {
			result.addCheck("agent", spec.Name, StatusWarning, fmt.Sprintf("agent %q has no tools: field — will use default tools only", spec.Name))
		}
	}

	if len(resolved.Bundle.AgentSpecs) == 0 {
		result.addCheck("agent", "", StatusError, "no agents found")
	}

	// Validate default_agent references an actual agent.
	if hasManifest && manifest.DefaultAgent != "" && !agentNames[manifest.DefaultAgent] {
		result.addCheck("manifest", "", StatusError, fmt.Sprintf("default_agent %q does not match any discovered agent: %v", manifest.DefaultAgent, sortedKeys(agentNames)))
	}

	// ── Skills ────────────────────────────────────────────────────────────

	// Collect local skills from discovery.
	localSkills := map[string]bool{}
	for _, sk := range resolved.Bundle.Skills {
		localSkills[sk.Name] = true
		result.Skills.Local = append(result.Skills.Local, sk.Name)
	}
	sort.Strings(result.Skills.Local)

	// Scan global skill directories.
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	globalSkills := map[string]bool{}
	if homeDir != "" {
		for _, base := range []string{".agents", ".claude"} {
			skillsDir := filepath.Join(homeDir, base, "skills")
			entries, err := os.ReadDir(skillsDir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					globalSkills[entry.Name()] = true
				}
			}
		}
	}
	for name := range globalSkills {
		result.Skills.GlobalAvailable = append(result.Skills.GlobalAvailable, name)
	}
	sort.Strings(result.Skills.GlobalAvailable)

	// Determine if global skills are included in discovery.
	globalIncluded := false
	if hasManifest && manifest.Discovery.IncludeGlobalUserResources != nil {
		globalIncluded = *manifest.Discovery.IncludeGlobalUserResources
	}
	result.Skills.GlobalIncluded = globalIncluded

	// Check agent skill references against discoverable skills.
	result.Skills.AgentSkillRefs = map[string][]string{}
	allDiscoverable := map[string]bool{}
	for k := range localSkills {
		allDiscoverable[k] = true
	}
	if globalIncluded {
		for k := range globalSkills {
			allDiscoverable[k] = true
		}
	}

	unresolvable := map[string]bool{}
	for _, spec := range resolved.Bundle.AgentSpecs {
		if len(spec.Skills) > 0 {
			result.Skills.AgentSkillRefs[spec.Name] = spec.Skills
		}
		for _, sk := range spec.Skills {
			if !allDiscoverable[sk] {
				unresolvable[sk] = true
			}
		}
	}
	for name := range unresolvable {
		result.Skills.Unresolvable = append(result.Skills.Unresolvable, name)
	}
	sort.Strings(result.Skills.Unresolvable)

	for _, name := range result.Skills.Unresolvable {
		if globalSkills[name] && !globalIncluded {
			result.addCheck("skill", name, StatusError, fmt.Sprintf("agent references skill %q which exists globally at ~/%s/skills/%s but include_global_user_resources is not enabled in the manifest", name, globalSkillBase(homeDir, name), name))
		} else {
			result.addCheck("skill", name, StatusError, fmt.Sprintf("agent references skill %q but it is not discoverable (not found locally or globally)", name))
		}
	}

	if len(globalSkills) > 0 && !globalIncluded {
		result.addCheck("skill", "", StatusWarning, fmt.Sprintf("global skills available (%s) but include_global_user_resources is not enabled", strings.Join(result.Skills.GlobalAvailable, ", ")))
	}

	// ── Workflows, commands, actions, triggers ────────────────────────────

	actionNames := map[string]bool{}
	for _, a := range resolved.Bundle.Actions {
		actionNames[a.Name] = true
		result.Actions = append(result.Actions, a.Name)
	}

	workflowNames := map[string]bool{}
	for _, w := range resolved.Bundle.Workflows {
		workflowNames[w.Name] = true
		result.Workflows = append(result.Workflows, w.Name)

		// Check workflow step action references.
		if steps, ok := w.Definition["steps"].([]any); ok {
			for _, step := range steps {
				if stepMap, ok := step.(map[string]any); ok {
					if actionRef, ok := stepMap["action"].(string); ok {
						if !actionNames[actionRef] {
							result.addCheck("workflow", w.Name, StatusWarning, fmt.Sprintf("workflow %q step references action %q which is not declared in actions/", w.Name, actionRef))
						}
					}
				}
			}
		}
	}

	for _, cmd := range resolved.Bundle.CommandResources {
		name := strings.Join(cmd.CommandPath, " ")
		if name == "" {
			name = cmd.Name
		}
		result.StructuredCommands = append(result.StructuredCommands, name)

		// Check command workflow/action targets.
		if cmd.Target.Workflow != "" && !workflowNames[cmd.Target.Workflow] {
			result.addCheck("command", name, StatusWarning, fmt.Sprintf("command %q targets workflow %q which is not declared", name, cmd.Target.Workflow))
		}
		if cmd.Target.Action != "" && !actionNames[cmd.Target.Action] {
			result.addCheck("command", name, StatusWarning, fmt.Sprintf("command %q targets action %q which is not declared", name, cmd.Target.Action))
		}
	}

	for _, cmd := range resolved.Bundle.Commands {
		result.Commands = append(result.Commands, cmd.Descriptor().Name)
	}

	for _, t := range resolved.Bundle.Triggers {
		result.Triggers = append(result.Triggers, t.Name)
	}

	sort.Strings(result.Workflows)
	sort.Strings(result.Commands)
	sort.Strings(result.StructuredCommands)
	sort.Strings(result.Actions)
	sort.Strings(result.Triggers)

	return result, nil
}

func (r *ValidationResult) addCheck(category, subject, status, message string) {
	r.Checks = append(r.Checks, Check{
		Category: category,
		Subject:  subject,
		Status:   status,
		Message:  message,
	})
}

// globalSkillBase returns the base directory name (.agents or .claude) where
// a global skill was found.
func globalSkillBase(homeDir, skillName string) string {
	for _, base := range []string{".claude", ".agents"} {
		skillDir := filepath.Join(homeDir, base, "skills", skillName)
		if info, err := os.Stat(skillDir); err == nil && info.IsDir() {
			return base
		}
	}
	return ".agents"
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
