package planner

import (
	"context"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

func (p *Planner) ContextProvider() agentcontext.Provider {
	return plannerProvider{planner: p}
}

type plannerProvider struct {
	planner *Planner
}

func (p plannerProvider) Key() agentcontext.ProviderKey {
	if p.planner == nil || p.planner.InstanceID() == "" {
		return "planner"
	}
	return agentcontext.ProviderKey("planner/" + p.planner.InstanceID())
}

func (p plannerProvider) GetContext(ctx context.Context, req agentcontext.Request) (agentcontext.ProviderContext, error) {
	if p.planner == nil {
		return agentcontext.ProviderContext{}, nil
	}
	plan, err := p.planner.State(ctx)
	if err != nil {
		return agentcontext.ProviderContext{}, err
	}
	if !p.planner.created {
		return agentcontext.ProviderContext{}, nil
	}

	fragments := []agentcontext.ContextFragment{{
		Key:       "planner/meta",
		Role:      unified.RoleUser,
		Authority: agentcontext.AuthorityUser,
		Content:   renderMeta(plan),
	}}
	for _, step := range plan.Steps {
		fragments = append(fragments, agentcontext.ContextFragment{
			Key:       agentcontext.FragmentKey("planner/step/" + step.ID),
			Role:      unified.RoleUser,
			Authority: agentcontext.AuthorityUser,
			Content:   renderStep(step),
		})
	}
	return agentcontext.ProviderContext{Fragments: fragments}, nil
}

func renderMeta(plan Plan) string {
	var current string
	if plan.CurrentStepID != "" {
		current = fmt.Sprintf("; current step: %s", plan.CurrentStepID)
	}
	title := plan.Title
	if title == "" {
		title = plan.ID
	}
	return fmt.Sprintf("Plan %q has %d step(s)%s.", title, len(plan.Steps), current)
}

func renderStep(step Step) string {
	parts := []string{
		fmt.Sprintf("id: %s", step.ID),
		fmt.Sprintf("status: %s", step.Status),
		fmt.Sprintf("title: %s", step.Title),
	}
	if step.ParentID != "" {
		parts = append(parts, fmt.Sprintf("parent: %s", step.ParentID))
	}
	if len(step.DependsOn) > 0 {
		parts = append(parts, fmt.Sprintf("depends_on: %s", strings.Join(step.DependsOn, ", ")))
	}
	return strings.Join(parts, "\n")
}
