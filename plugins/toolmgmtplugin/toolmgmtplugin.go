// Package toolmgmtplugin bundles the tool management tools (tools_list,
// tools_activate, tools_deactivate) and the active-tools context provider
// into a single [app.Plugin] implementation. It composes the existing
// [tools/toolmgmt] and [agentcontext/contextproviders] packages.
package toolmgmtplugin

import (
	"context"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
)

// Plugin bundles tool management tools and the active-tools context provider
// behind the app.Plugin interface.
type Plugin struct{}

// New creates a tool management plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the plugin identity.
func (p *Plugin) Name() string { return "toolmgmt" }

// Tools returns the tool management tools: tools_list, tools_activate,
// tools_deactivate.
func (p *Plugin) Tools() []tool.Tool {
	return toolmgmt.Tools()
}

// AgentContextProviders returns a lazy active-tools context provider that
// calls ActiveTools on each render, so runtime tool activation/deactivation
// changes are reflected in context.
func (p *Plugin) AgentContextProviders(info app.AgentContextInfo) []agentcontext.Provider {
	if info.ActiveTools == nil {
		return nil
	}
	return []agentcontext.Provider{
		&lazyToolsProvider{activeTools: info.ActiveTools},
	}
}

// lazyToolsProvider calls the ActiveTools closure on every GetContext so the
// rendered tool list reflects runtime activation changes. This avoids the
// snapshot-at-construction problem that contextproviders.Tools has.
type lazyToolsProvider struct {
	activeTools func() []tool.Tool
}

func (p *lazyToolsProvider) Key() agentcontext.ProviderKey { return "tools" }

func (p *lazyToolsProvider) GetContext(ctx context.Context, req agentcontext.Request) (agentcontext.ProviderContext, error) {
	// Delegate to the static Tools provider, but build it fresh each time
	// so the tool list is current.
	delegate := contextproviders.Tools(p.activeTools()...)
	return delegate.GetContext(ctx, req)
}
