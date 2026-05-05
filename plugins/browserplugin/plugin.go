package browserplugin

import (
	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/tool"
)

// Interface compliance.
var (
	_ app.Plugin             = (*Plugin)(nil)
	_ app.ActionsPlugin      = (*Plugin)(nil)
	_ app.CatalogToolsPlugin = (*Plugin)(nil)
	_ app.AgentContextPlugin = (*Plugin)(nil)
)

// Plugin provides browser automation via the Chrome DevTools Protocol.
type Plugin struct {
	sessions   *SessionManager
	actionList []action.Action
	browserT   tool.Tool
}

// New creates a browser plugin with the given options.
func New(opts ...Option) *Plugin {
	cfg := DefaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	p := &Plugin{
		sessions: NewSessionManager(cfg),
	}
	p.actionList = p.actions()
	p.browserT = p.browserTool()
	return p
}

// Name returns the plugin identity.
func (p *Plugin) Name() string { return "browser" }

// Actions returns the core browser actions for registration in the app's
// action registry. These are independently callable from YAML workflows and
// programmatic Go code.
func (p *Plugin) Actions() []action.Action { return p.actionList }

// CatalogTools returns the browser tool as a catalog tool (opt-in activation).
func (p *Plugin) CatalogTools() []tool.Tool { return []tool.Tool{p.browserT} }

// AgentContextProviders returns the browser context provider that injects
// the current page's interactable elements into the agent's context window.
func (p *Plugin) AgentContextProviders(_ app.AgentContextInfo) []agentcontext.Provider {
	return []agentcontext.Provider{
		&browserContextProvider{sessions: p.sessions},
	}
}

// Shutdown closes all browser sessions and stops the idle reaper.
func (p *Plugin) Shutdown() {
	p.sessions.CloseAll()
}
