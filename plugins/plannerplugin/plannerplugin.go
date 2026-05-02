// Package plannerplugin contributes the stateful planner capability factory.
package plannerplugin

import (
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
)

const PluginName = "planner"

// Plugin exposes the planner capability through the app plugin system.
type Plugin struct{}

// New creates a planner capability plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the plugin identity.
func (p *Plugin) Name() string { return PluginName }

// CapabilityFactories returns the planner capability factory.
func (p *Plugin) CapabilityFactories() []capability.Factory {
	return []capability.Factory{planner.Factory{}}
}
