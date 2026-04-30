// Package standard provides pre-assembled plugin sets for common agent
// configurations. It is the plugin-oriented counterpart to
// [tools/standard.Tools] — consumers using [app.App] should prefer
// [Plugins] over [tools/standard.Tools] to get both tools and context
// providers wired automatically.
//
// [Plugins] and [tools/standard.Tools] are parallel APIs. They must not be
// used together for the same domain (e.g. registering both gitplugin and
// git.Tools() would cause duplicate tool registrations).
package standard

import (
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/plugins/gitplugin"
	"github.com/codewandler/agentsdk/plugins/skillplugin"
	"github.com/codewandler/agentsdk/plugins/toolmgmtplugin"
)

// Options configures which plugins are included in the standard set.
type Options struct {
	// IncludeGit adds the git plugin (git_status, git_diff tools + git
	// context provider).
	IncludeGit bool

	// GitMode controls the git context provider mode. Defaults to
	// [contextproviders.GitMinimal] when IncludeGit is true.
	GitMode contextproviders.GitMode

	// IncludeProjectInventory adds a compact per-agent repository inventory
	// context provider. DefaultOptions enables it.
	IncludeProjectInventory bool
}

// DefaultOptions returns the default plugin set options. The default set
// includes skill, tool management, and project inventory plugins but not git
// (matching the default tool bundle behavior).
func DefaultOptions() Options {
	return Options{IncludeProjectInventory: true}
}

// Plugins returns the standard plugin set based on the given options.
func Plugins(opts Options) []app.Plugin {
	var out []app.Plugin
	out = append(out, skillplugin.New())
	out = append(out, toolmgmtplugin.New())
	if opts.IncludeProjectInventory {
		out = append(out, projectInventoryPlugin{})
	}
	if opts.IncludeGit {
		var gitOpts []gitplugin.Option
		if opts.GitMode != "" {
			gitOpts = append(gitOpts, gitplugin.WithMode(opts.GitMode))
		}
		out = append(out, gitplugin.New(gitOpts...))
	}
	return out
}

// DefaultPlugins returns the default plugin set for lightweight terminal
// agents.
func DefaultPlugins() []app.Plugin {
	return Plugins(DefaultOptions())
}

type projectInventoryPlugin struct{}

func (projectInventoryPlugin) Name() string { return "project_inventory" }

func (projectInventoryPlugin) AgentContextProviders(info app.AgentContextInfo) []agentcontext.Provider {
	if info.Workspace == "" {
		return []agentcontext.Provider{contextproviders.ProjectInventory()}
	}
	return []agentcontext.Provider{contextproviders.ProjectInventory(contextproviders.WithProjectInventoryWorkDir(info.Workspace))}
}
