package agent

import (
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
)

// BaselineProviderState carries the agent state needed to build baseline
// context providers. It is passed to [BaselineProviderFactory] so the factory
// can construct environment, time, model, tool, skill, and instruction
// providers without importing agent internals.
type BaselineProviderState struct {
	Workspace        string
	ResolvedModel    string
	ResolvedProvider string
	ContextWindow    int
	Effort           string
	ActiveTools      func() []tool.Tool
	SkillRepo        *skill.Repository
	SkillState       *skill.ActivationState
	InstructionPaths []string
}

// BaselineProviderFactory creates the baseline context providers for an agent
// session. The default factory is [DefaultBaselineProviders].
type BaselineProviderFactory func(BaselineProviderState) []agentcontext.Provider

// DefaultBaselineProviders returns the standard baseline context providers:
// environment, time, model identity, active tools, skill inventory, and
// AGENTS.md instruction paths.
func DefaultBaselineProviders(state BaselineProviderState) []agentcontext.Provider {
	var providers []agentcontext.Provider
	providers = append(providers, contextproviders.Environment(contextproviders.WithWorkDir(state.Workspace)))
	providers = append(providers, contextproviders.Time(time.Minute))
	providers = append(providers, contextproviders.Model(contextproviders.ModelInfo{
		Name:          state.ResolvedModel,
		Provider:      state.ResolvedProvider,
		ContextWindow: state.ContextWindow,
		Effort:        state.Effort,
	}))
	if state.ActiveTools != nil {
		providers = append(providers, contextproviders.Tools(state.ActiveTools()...))
	}
	if state.SkillRepo != nil || state.SkillState != nil {
		providers = append(providers, contextproviders.SkillInventoryProvider(contextproviders.SkillInventory{
			Catalog: state.SkillRepo,
			State:   state.SkillState,
		}))
	}
	if len(state.InstructionPaths) > 0 {
		providers = append(providers, contextproviders.AgentsMarkdown(state.InstructionPaths, contextproviders.AgentsMarkdownOption(contextproviders.WithFileWorkDir(state.Workspace))))
	}
	return providers
}
