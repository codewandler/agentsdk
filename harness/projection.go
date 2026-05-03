package harness

import (
	"fmt"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/tool"
)

// AgentProjection is a session-owned contribution to an agent-facing surface.
// It is intentionally not a plugin abstraction; it projects session capabilities
// onto a running agent without introducing a second harness plugin system.
type AgentProjection struct {
	Tools            []tool.Tool
	ContextProviders []agentcontext.Provider
}

// AgentCommandProjection returns the agent-facing command surface for this
// session: the command envelope tool plus catalog context.
func (s *Session) AgentCommandProjection() AgentProjection {
	if s == nil {
		return AgentProjection{}
	}
	return AgentProjection{
		Tools: []tool.Tool{
			s.agentCommandTool(),
		},
		ContextProviders: []agentcontext.Provider{
			s.agentCommandCatalogContextProvider(),
		},
	}
}

// AttachAgentProjection registers a session projection on the running agent.
// Registration is idempotent for existing tool names and provider keys.
func (s *Session) AttachAgentProjection(projection AgentProjection) error {
	if s == nil {
		return fmt.Errorf("harness: session is nil")
	}
	if s.Agent == nil {
		return fmt.Errorf("harness: agent is required")
	}
	if err := s.Agent.RegisterTools(projection.Tools...); err != nil {
		return err
	}
	if err := s.Agent.RegisterContextProviders(projection.ContextProviders...); err != nil {
		return err
	}
	return nil
}
