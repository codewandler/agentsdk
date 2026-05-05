// Package skillplugin bundles the skill activation tool and the skill
// inventory context provider into a single [app.Plugin] implementation.
// It composes the existing [tools/skills] and [agentcontext/contextproviders]
// packages.
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

// Plugin bundles the skill activation tool and the skill inventory context
// provider behind the app.Plugin interface.
type Plugin struct {
	sources []skill.Source
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

// SkillSources returns the configured skill sources.
func (p *Plugin) SkillSources() []skill.Source {
	return append([]skill.Source(nil), p.sources...)
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
