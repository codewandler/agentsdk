// Package appconfig provides a declarative YAML/JSON application configuration
// format. A single entry file (agentsdk.app.yaml or agentsdk.app.json) defines
// the application, with include globs to pull in additional resource files.
//
// Documents use a "kind" field to distinguish types:
//   - config (default): root application config with includes, resolution, plugins
//   - agent: agent specification
//   - command: command definition
//   - workflow: workflow definition
//   - action: action definition
//   - datasource: datasource definition
//   - trigger: trigger definition
//
// Multi-doc YAML is supported: multiple documents separated by "---" in one file.
package appconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/llmadapter/unified"
)

// EntryFileNames are the filenames searched for the root config entry.
var EntryFileNames = []string{"agentsdk.app.yaml", "agentsdk.app.yml", "agentsdk.app.json"}

// Kind identifies the document type.
type Kind string

const (
	KindConfig     Kind = "config"
	KindAgent      Kind = "agent"
	KindCommand    Kind = "command"
	KindWorkflow   Kind = "workflow"
	KindAction     Kind = "action"
	KindDatasource Kind = "datasource"
	KindTrigger    Kind = "trigger"
)

// Document is a single parsed document with a kind discriminator.
type Document struct {
	Kind Kind
	Raw  map[string]any
}

// Config is the root application configuration (kind=config).
type Config struct {
	Name         string            `yaml:"name,omitempty" json:"name,omitempty"`
	DefaultAgent string            `yaml:"default_agent,omitempty" json:"default_agent,omitempty"`
	Include      []string          `yaml:"include,omitempty" json:"include,omitempty"`
	Sources      []string          `yaml:"sources,omitempty" json:"sources,omitempty"`
	Resolution   *ResolutionConfig `yaml:"resolution,omitempty" json:"resolution,omitempty"`
	Discovery    *DiscoveryConfig  `yaml:"discovery,omitempty" json:"discovery,omitempty"`
	Plugins      []PluginRef       `yaml:"plugins,omitempty" json:"plugins,omitempty"`
}

// AgentDoc is an agent document (kind=agent).
type AgentDoc struct {
	Name         string   `yaml:"name" json:"name"`
	Description  string   `yaml:"description,omitempty" json:"description,omitempty"`
	Model        string   `yaml:"model,omitempty" json:"model,omitempty"`
	MaxTokens    int      `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	MaxSteps     int      `yaml:"max_steps,omitempty" json:"max_steps,omitempty"`
	Temperature  float64  `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	Thinking     string   `yaml:"thinking,omitempty" json:"thinking,omitempty"`
	Effort       string   `yaml:"effort,omitempty" json:"effort,omitempty"`
	Tools        []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	Skills       []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Commands     []string `yaml:"commands,omitempty" json:"commands,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	System       string   `yaml:"system,omitempty" json:"system,omitempty"`
}

// CommandDoc is a command document (kind=command).
type CommandDoc struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Target      *CommandTarget `yaml:"target,omitempty" json:"target,omitempty"`
}

// CommandTarget specifies what a command executes.
type CommandTarget struct {
	Workflow string `yaml:"workflow,omitempty" json:"workflow,omitempty"`
	Action   string `yaml:"action,omitempty" json:"action,omitempty"`
	Prompt   string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Input    any    `yaml:"input,omitempty" json:"input,omitempty"`
}

// WorkflowDoc is a workflow document (kind=workflow).
type WorkflowDoc struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Steps       []any  `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// ActionDoc is an action document (kind=action).
type ActionDoc struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Kind        string `yaml:"action_kind,omitempty" json:"action_kind,omitempty"`
}

// DatasourceDoc is a datasource document (kind=datasource).
type DatasourceDoc struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Kind        string `yaml:"datasource_kind,omitempty" json:"datasource_kind,omitempty"`
}

// TriggerDoc is a trigger document (kind=trigger).
type TriggerDoc struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Source      map[string]any `yaml:"source,omitempty" json:"source,omitempty"`
	Target      map[string]any `yaml:"target,omitempty" json:"target,omitempty"`
}

// ResolutionConfig configures resource resolution.
type ResolutionConfig struct {
	Precedence []string          `yaml:"precedence,omitempty" json:"precedence,omitempty"`
	Aliases    map[string]string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
}

// DiscoveryConfig configures resource discovery policy.
type DiscoveryConfig struct {
	IncludeGlobalUserResources bool `yaml:"include_global_user_resources,omitempty" json:"include_global_user_resources,omitempty"`
	IncludeExternalEcosystems  bool `yaml:"include_external_ecosystems,omitempty" json:"include_external_ecosystems,omitempty"`
	AllowRemote                bool `yaml:"allow_remote,omitempty" json:"allow_remote,omitempty"`
}

// PluginRef references a plugin to load.
type PluginRef struct {
	Name   string         `yaml:"name" json:"name"`
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// LoadResult holds all parsed documents from the entry file and its includes.
type LoadResult struct {
	Config     Config
	Agents     []AgentDoc
	Commands   []CommandDoc
	Workflows  []WorkflowDoc
	Actions    []ActionDoc
	Datasources []DatasourceDoc
	Triggers   []TriggerDoc
	EntryPath  string
}

// FindEntryFile searches dir for a known entry file name.
func FindEntryFile(dir string) (string, bool) {
	for _, name := range EntryFileNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

// Load finds the entry file in dir, parses it and all includes, and returns
// the merged LoadResult.
func Load(dir string) (LoadResult, error) {
	path, ok := FindEntryFile(dir)
	if !ok {
		return LoadResult{}, fmt.Errorf("appconfig: no entry file found in %s (tried %s)", dir, strings.Join(EntryFileNames, ", "))
	}
	return LoadFile(path)
}

// LoadFile parses the entry file and all its includes.
func LoadFile(path string) (LoadResult, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return LoadResult{}, err
	}
	result := LoadResult{EntryPath: absPath}
	if err := loadFileInto(&result, absPath); err != nil {
		return LoadResult{}, err
	}
	// Process includes from the config.
	baseDir := filepath.Dir(absPath)
	for _, pattern := range result.Config.Include {
		filePath, fragment := splitFragment(pattern)
		expanded := expandVars(filePath, baseDir)
		matches, err := filepath.Glob(expanded)
		if err != nil {
			return LoadResult{}, fmt.Errorf("appconfig: glob %q (expanded: %q): %w", pattern, expanded, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if match == absPath {
				continue // skip self
			}
			if fragment != nil {
				if err := loadFileFragmentInto(&result, match, fragment); err != nil {
					return LoadResult{}, fmt.Errorf("appconfig: include %s#%s: %w", match, strings.Join(fragment, "."), err)
				}
			} else {
				if err := loadFileInto(&result, match); err != nil {
					return LoadResult{}, fmt.Errorf("appconfig: include %s: %w", match, err)
				}
			}
		}
	}
	return result, nil
}

// ToAppOptions converts the LoadResult into []app.Option for app.New.
func (r LoadResult) ToAppOptions() []app.Option {
	var opts []app.Option
	for _, spec := range r.ToAgentSpecs() {
		opts = append(opts, app.WithAgentSpec(spec))
	}
	bundle := r.ToContributionBundle()
	if hasContributions(bundle) {
		opts = append(opts, app.WithResourceBundle(bundle))
	}
	if r.Config.DefaultAgent != "" {
		opts = append(opts, app.WithDefaultAgent(r.Config.DefaultAgent))
	}
	return opts
}

// ToAgentSpecs converts agent documents to agentconfig.Spec values.
func (r LoadResult) ToAgentSpecs() []agentconfig.Spec {
	specs := make([]agentconfig.Spec, 0, len(r.Agents))
	for _, agent := range r.Agents {
		spec := agentconfig.Spec{
			Name:        agent.Name,
			Description: agent.Description,
			System:      agent.System,
			Tools:       agent.Tools,
			Skills:      agent.Skills,
			Commands:    agent.Commands,
		}
		if agent.Model != "" || agent.MaxTokens > 0 || agent.Thinking != "" || agent.Effort != "" {
			spec.Inference = agentconfig.InferenceOptions{
				Model:       agent.Model,
				MaxTokens:   agent.MaxTokens,
				Temperature: agent.Temperature,
				Thinking:    agentconfig.ThinkingMode(agent.Thinking),
				Effort:      unified.ReasoningEffort(agent.Effort),
			}
		}
		if agent.MaxSteps > 0 {
			spec.MaxSteps = agent.MaxSteps
		}
		specs = append(specs, spec)
	}
	return specs
}

// ToContributionBundle converts resource documents into a ContributionBundle.
func (r LoadResult) ToContributionBundle() resource.ContributionBundle {
	var bundle resource.ContributionBundle
	origin := "config"
	ns := resource.NewNamespace()
	if r.Config.Name != "" {
		ns = resource.NewNamespace(r.Config.Name)
	}

	for _, wf := range r.Workflows {
		def := map[string]any{"description": wf.Description}
		if len(wf.Steps) > 0 {
			def["steps"] = wf.Steps
		}
		bundle.Workflows = append(bundle.Workflows, resource.WorkflowContribution{
			Name:        wf.Name,
			Description: wf.Description,
			Definition:  def,
			RID:         resource.ResourceID{Kind: "workflow", Origin: origin, Namespace: ns, Name: wf.Name},
		})
	}

	for _, act := range r.Actions {
		bundle.Actions = append(bundle.Actions, resource.ActionContribution{
			Name:        act.Name,
			Description: act.Description,
			Kind:        act.Kind,
			RID:         resource.ResourceID{Kind: "action", Origin: origin, Namespace: ns, Name: act.Name},
		})
	}

	for _, ds := range r.Datasources {
		bundle.DataSources = append(bundle.DataSources, resource.DataSourceContribution{
			Name:        ds.Name,
			Description: ds.Description,
			Kind:        ds.Kind,
			RID:         resource.ResourceID{Kind: "datasource", Origin: origin, Namespace: ns, Name: ds.Name},
		})
	}

	for _, tr := range r.Triggers {
		def := map[string]any{}
		if tr.Source != nil {
			def["source"] = tr.Source
		}
		if tr.Target != nil {
			def["target"] = tr.Target
		}
		bundle.Triggers = append(bundle.Triggers, resource.TriggerContribution{
			Name:        tr.Name,
			Description: tr.Description,
			Definition:  def,
			RID:         resource.ResourceID{Kind: "trigger", Origin: origin, Namespace: ns, Name: tr.Name},
		})
	}

	for _, cmd := range r.Commands {
		contrib := resource.CommandContribution{
			Name:        cmd.Name,
			Description: cmd.Description,
			CommandPath: []string{cmd.Name},
			RID:         resource.ResourceID{Kind: "command", Origin: origin, Namespace: ns, Name: cmd.Name},
		}
		if cmd.Target != nil {
			contrib.Target = resource.CommandTarget{
				Workflow: cmd.Target.Workflow,
				Action:   cmd.Target.Action,
				Prompt:   cmd.Target.Prompt,
				Input:    cmd.Target.Input,
			}
			switch {
			case cmd.Target.Workflow != "":
				contrib.Target.Kind = resource.CommandTargetWorkflow
			case cmd.Target.Action != "":
				contrib.Target.Kind = resource.CommandTargetAction
			case cmd.Target.Prompt != "":
				contrib.Target.Kind = resource.CommandTargetPrompt
			}
		}
		bundle.CommandResources = append(bundle.CommandResources, contrib)
	}

	return bundle
}

// loadFileFragmentInto loads a file, extracts a subtree via the dot-separated
// fragment path, and applies it as a single document.
func loadFileFragmentInto(result *LoadResult, path string, fragment []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	// Parse the whole file into a raw structure.
	var raw any
	if isJSON(data) {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse JSON %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse YAML %s: %w", path, err)
		}
	}
	extracted := extractFragment(raw, fragment)
	if extracted == nil {
		return fmt.Errorf("fragment %s not found in %s", strings.Join(fragment, "."), path)
	}
	doc := Document{Kind: docKind(extracted), Raw: extracted}
	return applyDocument(result, doc, path)
}

// loadFileInto parses a YAML/JSON file (supporting multi-doc YAML) and
// appends the parsed documents to the result.
func loadFileInto(result *LoadResult, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("appconfig: read %s: %w", path, err)
	}
	docs, err := parseDocuments(data, path)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if err := applyDocument(result, doc, path); err != nil {
			return fmt.Errorf("appconfig: %s: %w", path, err)
		}
	}
	return nil
}

// parseDocuments splits multi-doc YAML (or single JSON) into documents.
func parseDocuments(data []byte, source string) ([]Document, error) {
	// Try JSON first (single document).
	if isJSON(data) {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("appconfig: parse JSON %s: %w", source, err)
		}
		return []Document{{Kind: docKind(raw), Raw: raw}}, nil
	}
	// Multi-doc YAML.
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var docs []Document
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("appconfig: parse YAML %s: %w", source, err)
		}
		if raw == nil {
			continue
		}
		docs = append(docs, Document{Kind: docKind(raw), Raw: raw})
	}
	return docs, nil
}

// applyDocument routes a parsed document to the appropriate field in LoadResult.
func applyDocument(result *LoadResult, doc Document, source string) error {
	raw, err := yaml.Marshal(doc.Raw)
	if err != nil {
		return err
	}
	switch doc.Kind {
	case KindConfig:
		var cfg Config
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("config: %w", err)
		}
		result.Config = mergeConfig(result.Config, cfg)
	case KindAgent:
		var agent AgentDoc
		if err := yaml.Unmarshal(raw, &agent); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
		result.Agents = append(result.Agents, agent)
	case KindCommand:
		var cmd CommandDoc
		if err := yaml.Unmarshal(raw, &cmd); err != nil {
			return fmt.Errorf("command: %w", err)
		}
		result.Commands = append(result.Commands, cmd)
	case KindWorkflow:
		var wf WorkflowDoc
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			return fmt.Errorf("workflow: %w", err)
		}
		result.Workflows = append(result.Workflows, wf)
	case KindAction:
		var act ActionDoc
		if err := yaml.Unmarshal(raw, &act); err != nil {
			return fmt.Errorf("action: %w", err)
		}
		result.Actions = append(result.Actions, act)
	case KindDatasource:
		var ds DatasourceDoc
		if err := yaml.Unmarshal(raw, &ds); err != nil {
			return fmt.Errorf("datasource: %w", err)
		}
		result.Datasources = append(result.Datasources, ds)
	case KindTrigger:
		var tr TriggerDoc
		if err := yaml.Unmarshal(raw, &tr); err != nil {
			return fmt.Errorf("trigger: %w", err)
		}
		result.Triggers = append(result.Triggers, tr)
	default:
		return fmt.Errorf("unknown kind %q", doc.Kind)
	}
	return nil
}

func docKind(raw map[string]any) Kind {
	if k, ok := raw["kind"].(string); ok && k != "" {
		return Kind(k)
	}
	return KindConfig
}

func isJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// splitFragment splits a path#fragment reference. Returns the path and
// the dot-separated fragment segments (nil if no fragment).
func splitFragment(ref string) (string, []string) {
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		frag := ref[i+1:]
		if frag == "" {
			return ref[:i], nil
		}
		return ref[:i], strings.Split(frag, ".")
	}
	return ref, nil
}

// extractFragment walks a dot-separated path into a YAML structure and
// returns the subtree as a map. Returns nil if the path doesn't exist.
func extractFragment(root any, path []string) map[string]any {
	current := root
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[key]
		if !ok {
			return nil
		}
	}
	if m, ok := current.(map[string]any); ok {
		return m
	}
	return nil
}

// expandVars expands ~, $PWD, $HOME, and ${VAR} in a path pattern.
func expandVars(pattern string, baseDir string) string {
	if strings.HasPrefix(pattern, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			pattern = filepath.Join(home, pattern[2:])
		}
	}
	pattern = os.ExpandEnv(pattern)
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(baseDir, pattern)
	}
	return pattern
}

func mergeConfig(base, overlay Config) Config {
	if overlay.Name != "" {
		base.Name = overlay.Name
	}
	if overlay.DefaultAgent != "" {
		base.DefaultAgent = overlay.DefaultAgent
	}
	base.Include = append(base.Include, overlay.Include...)
	base.Sources = append(base.Sources, overlay.Sources...)
	base.Plugins = append(base.Plugins, overlay.Plugins...)
	if overlay.Resolution != nil {
		base.Resolution = overlay.Resolution
	}
	if overlay.Discovery != nil {
		base.Discovery = overlay.Discovery
	}
	return base
}

func hasContributions(b resource.ContributionBundle) bool {
	return len(b.Workflows) > 0 || len(b.Actions) > 0 || len(b.DataSources) > 0 ||
		len(b.Triggers) > 0 || len(b.CommandResources) > 0
}
