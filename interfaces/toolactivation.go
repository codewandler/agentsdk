package interfaces

import "github.com/codewandler/agentcore/tool"

// ActivationState manages which tools are active/inactive.
// Implemented by the agent runtime.
type ActivationState interface {
	// AllTools returns all registered tools.
	AllTools() []tool.Tool
	
	// ActiveTools returns currently active tools.
	ActiveTools() []tool.Tool
	
	// Activate makes tools matching patterns active, returns list of activated tool names.
	Activate(patterns ...string) []string
	
	// Deactivate makes tools matching patterns inactive, returns list of deactivated tool names.
	Deactivate(patterns ...string) []string
}
