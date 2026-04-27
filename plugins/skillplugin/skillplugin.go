// Package skillplugin bundles the skill activation tool, skill source
// discovery, and the skill inventory context provider into a single
// [app.Plugin] implementation. It composes the existing [tools/skills] and
// [agentcontext/contextproviders] packages.
package skillplugin

import (
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/skills"
)

// Option configures a Plugin.
type Option func(*Plugin)

// Plugin bundles the skill activation tool, skill source discovery, and the
// skill inventory context provider behind the app.Plugin interface.
type Plugin struct {
	discoveries    []app.SkillSourceDiscovery
	sources        []skill.Source
	discoveryErrors []error
}

// New creates a skill plugin with the given options.
func New(opts ...Option) *Plugin {
	p := &Plugin{}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// WithDiscovery adds a skill source discovery configuration. Discovered
// sources are contributed via [SkillsPlugin.SkillSources].
func WithDiscovery(discovery app.SkillSourceDiscovery) Option {
	return func(p *Plugin) { p.discoveries = append(p.discoveries, discovery) }
}

// WithSources adds explicit skill sources.
func WithSources(sources ...skill.Source) Option {
	return func(p *Plugin) { p.sources = append(p.sources, sources...) }
}

// Name returns the plugin identity.
func (p *Plugin) Name() string { return "skills" }

// Tools returns the skill activation tool.
func (p *Plugin) Tools() []tool.Tool {
	return skills.Tools()
}

// SkillSources returns discovered and explicit skill sources. Discovery
// errors are collected and available via [DiscoveryErrors].
func (p *Plugin) SkillSources() []skill.Source {
	var sources []skill.Source
	p.discoveryErrors = nil
	for _, d := range p.discoveries {
		discovered, err := app.DiscoverDefaultSkillSources(d)
		if err != nil {
			p.discoveryErrors = append(p.discoveryErrors, err)
			continue
		}
		sources = append(sources, discovered...)
	}
	sources = append(sources, p.sources...)
	return sources
}

// DiscoveryErrors returns errors from the most recent [SkillSources] call.
// Returns nil when all discoveries succeeded.
func (p *Plugin) DiscoveryErrors() []error {
	return append([]error(nil), p.discoveryErrors...)
}

// AgentContextProviders returns the skill inventory context provider using
// per-agent skill repository and activation state.
func (p *Plugin) AgentContextProviders(info app.AgentContextInfo) []agentcontext.Provider {
	if info.SkillRepository == nil && info.SkillState == nil {
		return nil
	}
	return []agentcontext.Provider{
		contextproviders.SkillInventoryProvider(contextproviders.SkillInventory{
			Catalog: info.SkillRepository,
			State:   info.SkillState,
		}),
	}
}
