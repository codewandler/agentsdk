// Package standard assembles common agentsdk tool bundles.
package standard

import (
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/filesystem"
	"github.com/codewandler/agentsdk/tools/git"
	"github.com/codewandler/agentsdk/tools/notify"
	"github.com/codewandler/agentsdk/tools/shell"
	"github.com/codewandler/agentsdk/tools/todo"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/codewandler/agentsdk/tools/turn"
	"github.com/codewandler/agentsdk/tools/web"
	"github.com/codewandler/agentsdk/websearch"
)

// Options configures a standard tool bundle.
type Options struct {
	WebSearchProvider websearch.Provider

	IncludeGit            bool
	IncludeNotify         bool
	IncludeTodo           bool
	IncludeToolManagement bool
	IncludeTurnDone       bool
}

// Tools returns the common coding-agent tools plus optional extras.
func Tools(opts Options) []tool.Tool {
	var out []tool.Tool
	out = append(out, shell.Tools()...)
	out = append(out, filesystem.Tools()...)
	out = append(out, web.Tools(opts.WebSearchProvider)...)
	if opts.IncludeGit {
		out = append(out, git.Tools()...)
	}
	if opts.IncludeNotify {
		out = append(out, notify.Tools()...)
	}
	if opts.IncludeTodo {
		out = append(out, todo.Tools()...)
	}
	if opts.IncludeToolManagement {
		out = append(out, toolmgmt.Tools()...)
	}
	if opts.IncludeTurnDone {
		out = append(out, turn.Tools()...)
	}
	return out
}

// DefaultTools returns the default bundle used by lightweight terminal agents.
func DefaultTools() []tool.Tool {
	return Tools(Options{
		WebSearchProvider:     web.DefaultSearchProviderFromEnv(),
		IncludeToolManagement: true,
	})
}
