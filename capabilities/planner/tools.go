package planner

import "github.com/codewandler/agentsdk/tool"

type ToolInput struct {
	Actions []Action `json:"actions" jsonschema:"required,minItems=1"`
}

func (p *Planner) Tools() []tool.Tool {
	return []tool.Tool{
		tool.New[ToolInput](
			"plan",
			"Create and update the session plan with an all-or-nothing batch of actions.",
			func(ctx tool.Ctx, input ToolInput) (tool.Result, error) {
				result, err := p.ApplyActions(ctx, input.Actions)
				if err != nil {
					return nil, err
				}
				raw, err := marshalResult(result)
				if err != nil {
					return nil, err
				}
				return tool.Text(string(raw)), nil
			},
			tool.WithGuidance[ToolInput]("Use plan to create or update the active plan. Batch related changes in one call; the batch is applied all-or-nothing."),
		),
	}
}
