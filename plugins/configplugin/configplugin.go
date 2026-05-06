// Package configplugin provides the /config session command as a plugin.
// It exposes the same functionality as "agentsdk config print" within
// an interactive session.
package configplugin

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/appconfig"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/markdown"
)

// Plugin implements app.Plugin and app.CommandsPlugin to expose /config.
type Plugin struct {
	workspace string
}

// Option configures the plugin.
type Option func(*Plugin)

// WithWorkspace sets the workspace directory for config discovery.
func WithWorkspace(dir string) Option {
	return func(p *Plugin) { p.workspace = dir }
}

// New creates a config plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{workspace: "."}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

func (p *Plugin) Name() string { return "config" }

func (p *Plugin) Commands() []command.Command {
	tree, err := command.NewTree("config", command.Description("Inspect application configuration")).
		Sub("print", command.Typed(p.printCommand),
			command.Description("Print the expanded configuration as YAML"),
		).
		Sub("validate", command.Typed(p.validateCommand),
			command.Description("Validate configuration structure"),
		).
		Build()
	if err != nil {
		return nil
	}
	return []command.Command{tree}
}

type configPrintInput struct{}
type configValidateInput struct{}

func (p *Plugin) printCommand(_ context.Context, _ configPrintInput) (command.Result, error) {
	result, err := p.loadConfig()
	if err != nil {
		return command.Result{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Config: %s\n\n", result.EntryPath)
	fmt.Fprintln(&b, "```yaml")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(result.Config); err != nil {
		return command.Result{}, err
	}
	fmt.Fprintln(&b, "```")
	rendered, err := markdown.RenderString(b.String())
	if err != nil {
		rendered = b.String()
	}
	return command.Display(command.TextPayload{Text: rendered}), nil
}

func (p *Plugin) validateCommand(_ context.Context, _ configValidateInput) (command.Result, error) {
	result, err := p.loadConfig()
	if err != nil {
		return command.Result{}, fmt.Errorf("config validation failed: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Config: %s\n", result.EntryPath)
	fmt.Fprintf(&b, "Name: %s\n", result.Config.Name)
	fmt.Fprintf(&b, "Default agent: %s\n", result.Config.DefaultAgent)
	fmt.Fprintf(&b, "Agents: %d\n", len(result.Agents))
	fmt.Fprintf(&b, "Commands: %d\n", len(result.Commands))
	fmt.Fprintf(&b, "Workflows: %d\n", len(result.Workflows))
	fmt.Fprintf(&b, "Actions: %d\n", len(result.Actions))
	fmt.Fprintf(&b, "Datasources: %d\n", len(result.Datasources))
	fmt.Fprintf(&b, "Triggers: %d\n", len(result.Triggers))
	fmt.Fprintf(&b, "Includes: %d\n", len(result.Config.Include))
	fmt.Fprintf(&b, "Plugins: %d\n", len(result.Config.Plugins))
	fmt.Fprintln(&b, "\n✓ configuration is valid")
	return command.Display(command.TextPayload{Text: b.String()}), nil
}

func (p *Plugin) loadConfig() (appconfig.LoadResult, error) {
	dir := p.workspace
	if dir == "" {
		dir = "."
	}
	return appconfig.Load(dir)
}



// Compile-time interface checks.
var (
	_ app.Plugin         = (*Plugin)(nil)
	_ app.CommandsPlugin = (*Plugin)(nil)
)
