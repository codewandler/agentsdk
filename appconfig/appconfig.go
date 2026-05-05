// Package appconfig provides a declarative YAML-based application
// configuration format as an alternative to the agentdir directory convention.
//
// A single YAML file (or directory of YAML files) can define agents, commands,
// workflows, actions, skills, datasources, triggers, and resolution settings.
// The loader produces []app.Option that can be passed to app.New alongside
// or instead of agentdir-loaded resource bundles.
package appconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/llmadapter/unified"
)

// Config is the top-level declarative application configuration.
type Config struct {
	// Name is the application/project name used for resource namespacing.
	Name string `yaml:"name,omitempty"`

	// DefaultAgent is the agent to use when none is specified.
	DefaultAgent string `yaml:"default_agent,omitempty"`

	// Sources lists agentdir roots to load (e.g. [".agents", "~/.agents"]).
	// These are loaded via the agentdir loader, not parsed by appconfig.
	Sources []string `yaml:"sources,omitempty"`

	// Agents defines agent specs inline.
	Agents map[string]AgentConfig `yaml:"agents,omitempty"`

	// Commands defines commands inline.
	Commands map[string]CommandConfig `yaml:"commands,omitempty"`

	// Workflows defines workflows inline.
	Workflows map[string]WorkflowConfig `yaml:"workflows,omitempty"`

	// Actions defines actions inline.
	Actions map[string]ActionConfig `yaml:"actions,omitempty"`

	// Datasources defines datasources inline.
	Datasources map[string]DatasourceConfig `yaml:"datasources,omitempty"`

	// Triggers defines triggers inline.
	Triggers map[string]TriggerConfig `yaml:"triggers,omitempty"`

	// Resolution configures resource resolution behavior.
	Resolution *ResolutionConfig `yaml:"resolution,omitempty"`

	// Discovery configures resource discovery policy.
	Discovery *DiscoveryConfig `yaml:"discovery,omitempty"`

	// Plugins lists plugin references to load.
	Plugins []PluginRef `yaml:"plugins,omitempty"`
}

// AgentConfig defines an agent inline in the config.
type AgentConfig struct {
	Description  string   `yaml:"description,omitempty"`
	Model        string   `yaml:"model,omitempty"`
	MaxTokens    int      `yaml:"max_tokens,omitempty"`
	MaxSteps     int      `yaml:"max_steps,omitempty"`
	Temperature  float64  `yaml:"temperature,omitempty"`
	Thinking     string   `yaml:"thinking,omitempty"`
	Effort       string   `yaml:"effort,omitempty"`
	Tools        []string `yaml:"tools,omitempty"`
	Skills       []string `yaml:"skills,omitempty"`
	Commands     []string `yaml:"commands,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty"`
	System       string   `yaml:"system,omitempty"`
}

// CommandConfig defines a command inline in the config.
type CommandConfig struct {
	Description string         `yaml:"description,omitempty"`
	Target      *CommandTarget `yaml:"target,omitempty"`
}

// CommandTarget specifies what a command executes.
type CommandTarget struct {
	Workflow string `yaml:"workflow,omitempty"`
	Action   string `yaml:"action,omitempty"`
	Prompt   string `yaml:"prompt,omitempty"`
	Input    any    `yaml:"input,omitempty"`
}

// WorkflowConfig defines a workflow inline in the config.
type WorkflowConfig struct {
	Description string `yaml:"description,omitempty"`
	Steps       []any  `yaml:"steps,omitempty"`
}

// ActionConfig defines an action inline in the config.
type ActionConfig struct {
	Description string `yaml:"description,omitempty"`
	Kind        string `yaml:"kind,omitempty"`
}

// DatasourceConfig defines a datasource inline in the config.
type DatasourceConfig struct {
	Description string `yaml:"description,omitempty"`
	Kind        string `yaml:"kind,omitempty"`
}

// TriggerConfig defines a trigger inline in the config.
type TriggerConfig struct {
	Description string         `yaml:"description,omitempty"`
	Source      map[string]any `yaml:"source,omitempty"`
	Target      map[string]any `yaml:"target,omitempty"`
}

// ResolutionConfig configures resource resolution.
type ResolutionConfig struct {
	Precedence []string          `yaml:"precedence,omitempty"`
	Aliases    map[string]string `yaml:"aliases,omitempty"`
}

// DiscoveryConfig configures resource discovery policy.
type DiscoveryConfig struct {
	IncludeGlobalUserResources bool `yaml:"include_global_user_resources,omitempty"`
	IncludeExternalEcosystems  bool `yaml:"include_external_ecosystems,omitempty"`
	AllowRemote                bool `yaml:"allow_remote,omitempty"`
}

// PluginRef references a plugin to load.
type PluginRef struct {
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config,omitempty"`
}

// LoadFile reads a YAML config file and returns the parsed Config.
func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("appconfig: read %s: %w", path, err)
	}
	return Parse(data, path)
}

// Parse parses YAML data into a Config.
func Parse(data []byte, source string) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("appconfig: parse %s: %w", source, err)
	}
	return cfg, nil
}

// LoadDir reads all YAML files in a directory and merges them into a
// single Config. Files are processed in lexicographic order.
func LoadDir(dir string) (Config, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Config{}, fmt.Errorf("appconfig: read dir %s: %w", dir, err)
	}
	var merged Config
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isYAMLFile(name) {
			continue
		}
		cfg, err := LoadFile(filepath.Join(dir, name))
		if err != nil {
			return Config{}, err
		}
		merged = mergeConfig(merged, cfg)
	}
	return merged, nil
}

// ToAppOptions converts the Config into []app.Option suitable for app.New.
// This includes agent specs, the contribution bundle, and the default agent.
// Agentdir sources listed in Config.Sources are NOT loaded here — the caller
// is responsible for loading them separately.
func (c Config) ToAppOptions() []app.Option {
	var opts []app.Option
	for _, spec := range c.ToAgentSpecs() {
		opts = append(opts, app.WithAgentSpec(spec))
	}
	bundle := c.ToContributionBundle()
	if len(bundle.Workflows) > 0 || len(bundle.Actions) > 0 || len(bundle.DataSources) > 0 ||
		len(bundle.Triggers) > 0 || len(bundle.CommandResources) > 0 {
		opts = append(opts, app.WithResourceBundle(bundle))
	}
	if c.DefaultAgent != "" {
		opts = append(opts, app.WithDefaultAgent(c.DefaultAgent))
	}
	return opts
}

// ToAgentSpecs converts inline agent definitions to agentconfig.Spec values.
func (c Config) ToAgentSpecs() []agentconfig.Spec {
	var specs []agentconfig.Spec
	for name, agent := range c.Agents {
		spec := agentconfig.Spec{
			Name:        name,
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

// ToContributionBundle converts inline resource definitions from the Config
// into a ContributionBundle. Agentdir sources are NOT loaded here — they
// should be loaded separately via the agentdir loader.
func (c Config) ToContributionBundle() resource.ContributionBundle {
	var bundle resource.ContributionBundle
	origin := "config"
	ns := resource.NewNamespace()
	if c.Name != "" {
		ns = resource.NewNamespace(c.Name)
	}

	for name, wf := range c.Workflows {
		def := map[string]any{"description": wf.Description}
		if len(wf.Steps) > 0 {
			def["steps"] = wf.Steps
		}
		bundle.Workflows = append(bundle.Workflows, resource.WorkflowContribution{
			Name:       name,
			Description: wf.Description,
			Definition: def,
			RID:        resource.ResourceID{Kind: "workflow", Origin: origin, Namespace: ns, Name: name},
		})
	}

	for name, act := range c.Actions {
		bundle.Actions = append(bundle.Actions, resource.ActionContribution{
			Name:        name,
			Description: act.Description,
			Kind:        act.Kind,
			RID:         resource.ResourceID{Kind: "action", Origin: origin, Namespace: ns, Name: name},
		})
	}

	for name, ds := range c.Datasources {
		bundle.DataSources = append(bundle.DataSources, resource.DataSourceContribution{
			Name:        name,
			Description: ds.Description,
			Kind:        ds.Kind,
			RID:         resource.ResourceID{Kind: "datasource", Origin: origin, Namespace: ns, Name: name},
		})
	}

	for name, tr := range c.Triggers {
		def := map[string]any{}
		if tr.Source != nil {
			def["source"] = tr.Source
		}
		if tr.Target != nil {
			def["target"] = tr.Target
		}
		bundle.Triggers = append(bundle.Triggers, resource.TriggerContribution{
			Name:        name,
			Description: tr.Description,
			Definition:  def,
			RID:         resource.ResourceID{Kind: "trigger", Origin: origin, Namespace: ns, Name: name},
		})
	}

	for name, cmd := range c.Commands {
		contrib := resource.CommandContribution{
			Name:        name,
			Description: cmd.Description,
			CommandPath: []string{name},
			RID:         resource.ResourceID{Kind: "command", Origin: origin, Namespace: ns, Name: name},
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

func isYAMLFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func mergeConfig(base, overlay Config) Config {
	if overlay.Name != "" {
		base.Name = overlay.Name
	}
	if overlay.DefaultAgent != "" {
		base.DefaultAgent = overlay.DefaultAgent
	}
	base.Sources = append(base.Sources, overlay.Sources...)
	if base.Agents == nil && len(overlay.Agents) > 0 {
		base.Agents = make(map[string]AgentConfig)
	}
	for k, v := range overlay.Agents {
		base.Agents[k] = v
	}
	if base.Commands == nil && len(overlay.Commands) > 0 {
		base.Commands = make(map[string]CommandConfig)
	}
	for k, v := range overlay.Commands {
		base.Commands[k] = v
	}
	if base.Workflows == nil && len(overlay.Workflows) > 0 {
		base.Workflows = make(map[string]WorkflowConfig)
	}
	for k, v := range overlay.Workflows {
		base.Workflows[k] = v
	}
	if base.Actions == nil && len(overlay.Actions) > 0 {
		base.Actions = make(map[string]ActionConfig)
	}
	for k, v := range overlay.Actions {
		base.Actions[k] = v
	}
	if base.Datasources == nil && len(overlay.Datasources) > 0 {
		base.Datasources = make(map[string]DatasourceConfig)
	}
	for k, v := range overlay.Datasources {
		base.Datasources[k] = v
	}
	if base.Triggers == nil && len(overlay.Triggers) > 0 {
		base.Triggers = make(map[string]TriggerConfig)
	}
	for k, v := range overlay.Triggers {
		base.Triggers[k] = v
	}
	base.Plugins = append(base.Plugins, overlay.Plugins...)
	if overlay.Resolution != nil {
		base.Resolution = overlay.Resolution
	}
	if overlay.Discovery != nil {
		base.Discovery = overlay.Discovery
	}
	return base
}
