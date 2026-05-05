// Package localcli defines the built-in local terminal plugin.
package localcli

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/plugins/browserplugin"
	"github.com/codewandler/agentsdk/plugins/plannerplugin"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/filesystem"
	"github.com/codewandler/agentsdk/tools/git"
	"github.com/codewandler/agentsdk/tools/jsonquery"
	"github.com/codewandler/agentsdk/tools/notify"
	"github.com/codewandler/agentsdk/tools/phone"
	"github.com/codewandler/agentsdk/tools/shell"
	"github.com/codewandler/agentsdk/tools/skills"
	"github.com/codewandler/agentsdk/tools/todo"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/codewandler/agentsdk/tools/turn"
	"github.com/codewandler/agentsdk/tools/vision"
	"github.com/codewandler/agentsdk/tools/web"
	"github.com/codewandler/agentsdk/websearch"
	"github.com/codewandler/cmdrisk"
)

const PluginName = "local_cli"

// Plugin contributes the local terminal tool catalog/defaults and planner
// capability factory. It is a named use-case/environment plugin, not a generic
// standard bundle.
type Plugin struct{}

type ToolOptions struct {
	WebSearchProvider websearch.Provider
	RiskAnalyzer      *cmdrisk.Analyzer
	PhoneConfig       *phone.Config
}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return PluginName }

func (p *Plugin) DefaultTools() []tool.Tool {
	return localTools(ToolOptions{WebSearchProvider: web.DefaultSearchProviderFromEnv()}, localToolSet{
		Git:            true,
		ToolManagement: true,
	})
}

func (p *Plugin) CatalogTools() []tool.Tool {
	opts := ToolOptions{WebSearchProvider: web.DefaultSearchProviderFromEnv(), PhoneConfig: &phone.Config{}}
	tools := localTools(opts, localToolSet{
		Git:            true,
		Notify:         true,
		Todo:           true,
		ToolManagement: true,
		TurnDone:       true,
		Vision:         true,
	})
	if opts.WebSearchProvider == nil {
		tools = append(tools, web.SearchTool(nil))
	}
	return tools
}

func (p *Plugin) CapabilityFactories() []capability.Factory {
	return plannerplugin.New().CapabilityFactories()
}

// Factory creates built-in plugins available to the local CLI host. The caller
// chooses which names are active through app/resource config or CLI flags.
type Factory struct{}

func NewFactory() Factory { return Factory{} }

func (Factory) PluginForName(_ context.Context, name string, config map[string]any) (app.Plugin, error) {
	switch name {
	case PluginName:
		return New(), nil
	case plannerplugin.PluginName:
		return plannerplugin.New(), nil
	case "browser":
		return newBrowserPlugin(config), nil
	default:
		return nil, fmt.Errorf("localcli: plugin %q not registered", name)
	}
}

func newBrowserPlugin(config map[string]any) *browserplugin.Plugin {
	var opts []browserplugin.Option
	if url, ok := config["remote_url"].(string); ok && url != "" {
		opts = append(opts, browserplugin.WithMode(browserplugin.ModeAttach))
		opts = append(opts, browserplugin.WithRemoteURL(url))
	}
	if headless, ok := config["headless"].(bool); ok {
		opts = append(opts, browserplugin.WithHeadless(headless))
	}
	if path, ok := config["chrome_path"].(string); ok && path != "" {
		opts = append(opts, browserplugin.WithChromePath(path))
	}
	return browserplugin.New(opts...)
}

// PluginForName creates a built-in plugin available to the local CLI host.
func PluginForName(ctx context.Context, name string, config map[string]any) (app.Plugin, error) {
	return NewFactory().PluginForName(ctx, name, config)
}

type localToolSet struct {
	Git            bool
	Notify         bool
	Todo           bool
	ToolManagement bool
	TurnDone       bool
	Vision         bool
}

func localTools(opts ToolOptions, set localToolSet) []tool.Tool {
	analyzer := opts.RiskAnalyzer
	if analyzer == nil {
		analyzer = cmdrisk.New(cmdrisk.Config{})
	}
	var out []tool.Tool
	out = append(out, shell.Tools(shell.WithRiskAnalyzer(analyzer))...)
	out = append(out, filesystem.Tools()...)
	out = append(out, jsonquery.Tools()...)
	out = append(out, web.Tools(opts.WebSearchProvider)...)
	out = append(out, skills.Tools()...)
	if set.Git {
		out = append(out, git.Tools()...)
	}
	if set.Notify {
		out = append(out, notify.Tools()...)
	}
	if set.Todo {
		out = append(out, todo.Tools()...)
	}
	if set.ToolManagement {
		out = append(out, toolmgmt.Tools()...)
	}
	if set.TurnDone {
		out = append(out, turn.Tools()...)
	}
	if set.Vision {
		out = append(out, vision.Tools(vision.ClientFromEnv())...)
	}
	if opts.PhoneConfig != nil {
		out = append(out, phone.Tools(*opts.PhoneConfig)...)
	}
	return out
}

// DefaultAgent returns the fallback terminal agent spec used when no app
// resources define an agent.
func DefaultAgent() agentconfig.Spec {
	return agentconfig.Spec{
		Name:        "default",
		Description: "Local CLI development assistant",
		System: `You are a concise, practical software agent running in a terminal.

Help the user inspect, explain, edit, and verify work in the current workspace.
Prefer direct, actionable answers. When changing code, keep edits scoped,
respect the existing project style, and verify with relevant tests or commands
when practical. If a request is ambiguous, make a reasonable assumption and
state it briefly.

When a task involves more than a couple of steps, use the plan tool to create a
plan before you start working. Mark each step in_progress as you begin it and
completed when it is done. For simple, single-action requests (a quick lookup,
one file edit, a short explanation) skip the plan and just act.`,
		Capabilities: []capability.AttachSpec{{
			CapabilityName: planner.CapabilityName,
			InstanceID:     "default",
		}},
	}
}
