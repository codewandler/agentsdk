package actiontooladapter

import (
	"encoding/json"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/tool"
)

type NormalizeInput struct {
	Text string `json:"text" jsonschema_description:"Text to trim and lowercase."`
}

type NormalizeOutput struct {
	Normalized string `json:"normalized"`
}

func NormalizeAction() action.Action {
	return action.NewTyped[NormalizeInput, NormalizeOutput](action.Spec{
		Name:        "normalize_text",
		Description: "Trim and lowercase text.",
	}, func(_ action.Ctx, input NormalizeInput) (NormalizeOutput, error) {
		return NormalizeOutput{Normalized: strings.ToLower(strings.TrimSpace(input.Text))}, nil
	})
}

func NormalizeTool() tool.Tool {
	return tool.FromAction(NormalizeAction(), tool.WithActionGuidance("Use when text normalization is useful before comparison."))
}

func ExecuteNormalize(t tool.Tool, ctx tool.Ctx, text string) (NormalizeOutput, error) {
	input, err := json.Marshal(NormalizeInput{Text: text})
	if err != nil {
		return NormalizeOutput{}, err
	}
	result, err := t.Execute(ctx, input)
	if err != nil {
		return NormalizeOutput{}, err
	}
	var out NormalizeOutput
	if err := json.Unmarshal([]byte(result.String()), &out); err != nil {
		return NormalizeOutput{}, err
	}
	return out, nil
}
