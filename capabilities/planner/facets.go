package planner

import (
	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
)

const ActionApplyActions = "planner.apply_actions"

func (p *Planner) Actions() []action.Action {
	return []action.Action{
		action.NewTyped[ToolInput, ApplyActionsResult](action.Spec{
			Name:        ActionApplyActions,
			Description: "Apply a batch of planner actions to the active plan.",
		}, func(ctx action.Ctx, input ToolInput) (ApplyActionsResult, error) {
			return p.ApplyActions(ctx, input.Actions)
		}),
	}
}

func (p plannerProvider) Descriptor() agentcontext.ProviderDescriptor {
	return agentcontext.ProviderDescriptor{
		Key:         p.Key(),
		Description: "active planner state and step list",
		Lifecycle:   "capability",
		Scope:       agentcontext.CacheTurn,
		CachePolicy: agentcontext.CachePolicy{Scope: agentcontext.CacheTurn},
	}
}
