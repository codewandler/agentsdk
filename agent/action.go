package agent

import (
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/llmadapter/unified"
)

const DefaultTurnActionName = "agent.turn"

// TurnAction exposes an agent turn as an action.Action. It is a narrow adapter
// for workflows and app code that need to call the current agent/session through
// the action layer while agent.Instance remains the lifecycle facade.
func TurnAction(inst *Instance, spec action.Spec) action.Action {
	if spec.Name == "" {
		spec.Name = DefaultTurnActionName
	}
	if spec.Description == "" {
		spec.Description = "Run a model turn with an agent"
	}
	if spec.Input.IsZero() {
		spec.Input = action.TypeOf[string]()
	}
	if spec.Output.IsZero() {
		spec.Output = action.TypeOf[string]()
	}
	return action.New(spec, func(ctx action.Ctx, input any) action.Result {
		if inst == nil {
			return action.Result{Error: fmt.Errorf("agent: turn action instance is nil")}
		}
		prompt, err := action.CastInput[string](input)
		if err != nil {
			return action.Result{Error: err}
		}
		output, err := inst.runTurnText(ctx, 0, prompt)
		if err != nil {
			return action.Result{Error: err}
		}
		return action.Result{Data: output}
	})
}

// TurnAction exposes this instance as an action.Action using the supplied spec.
// runTurnText runs an agent turn and returns the latest assistant text projected
// from conversation history after the turn commits.
func (a *Instance) runTurnText(ctx action.Ctx, turnID int, task string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("agent: instance is nil")
	}
	if err := a.RunTurn(ctx, turnID, task); err != nil {
		return "", err
	}
	return a.lastAssistantText()
}

func (a *Instance) lastAssistantText() (string, error) {
	if a == nil || a.runtime == nil || a.runtime.History() == nil {
		return "", fmt.Errorf("agent: runtime history is not initialized")
	}
	messages, err := a.runtime.History().Messages()
	if err != nil {
		return "", err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == unified.RoleAssistant {
			return textFromContentParts(messages[i].Content), nil
		}
	}
	return "", fmt.Errorf("agent: assistant response not found")
}

func textFromContentParts(parts []unified.ContentPart) string {
	var out []string
	for _, part := range parts {
		switch p := part.(type) {
		case unified.TextPart:
			if p.Text != "" {
				out = append(out, p.Text)
			}
		case unified.RefusalPart:
			if p.Text != "" {
				out = append(out, p.Text)
			}
		}
	}
	return strings.Join(out, "")
}
