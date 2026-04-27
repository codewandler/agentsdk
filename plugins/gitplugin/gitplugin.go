// Package gitplugin bundles git tools and the git context provider into a
// single [app.Plugin] implementation. It composes the existing [tools/git] and
// [agentcontext/contextproviders] packages — those remain importable for
// consumers who don't want the plugin abstraction.
package gitplugin

import (
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/git"
)

// Option configures a Plugin.
type Option func(*Plugin)

// Plugin bundles git tools (git_status, git_diff) and the git context provider
// behind the app.Plugin interface.
type Plugin struct {
	gitMode contextproviders.GitMode
	workDir string
	gitOpts []contextproviders.GitOption
}

// New creates a git plugin with the given options.
func New(opts ...Option) *Plugin {
	p := &Plugin{gitMode: contextproviders.GitMinimal}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// WithMode sets the git context provider mode (off, minimal, changed_files).
func WithMode(mode contextproviders.GitMode) Option {
	return func(p *Plugin) { p.gitMode = mode }
}

// WithWorkDir sets the working directory for the git context provider.
func WithWorkDir(dir string) Option {
	return func(p *Plugin) { p.workDir = dir }
}

// WithGitOption appends a raw [contextproviders.GitOption] for advanced
// configuration (timeout, max files, max bytes, custom runner).
func WithGitOption(opt contextproviders.GitOption) Option {
	return func(p *Plugin) { p.gitOpts = append(p.gitOpts, opt) }
}

// Name returns the plugin identity.
func (p *Plugin) Name() string { return "git" }

// Tools returns the git tools: git_status, git_diff.
func (p *Plugin) Tools() []tool.Tool {
	return git.Tools()
}

// ContextProviders returns the git context provider configured with the
// plugin's mode and work directory.
func (p *Plugin) ContextProviders() []agentcontext.Provider {
	opts := []contextproviders.GitOption{
		contextproviders.WithGitMode(p.gitMode),
	}
	if p.workDir != "" {
		opts = append(opts, contextproviders.WithGitWorkDir(p.workDir))
	}
	opts = append(opts, p.gitOpts...)
	return []agentcontext.Provider{contextproviders.Git(opts...)}
}
