// Package turn provides the turn_done tool and its sentinel result type.
package turn

import (
	"encoding/json"
	"fmt"

	"github.com/codewandler/core/tool"
)

// TurnDoneParams are the parameters for the turn_done tool.
type TurnDoneParams struct {
	// Content is rich response content (text, JSON, YAML, etc.).
	Content string `json:"content,omitempty" jsonschema:"description=Rich response content (text. data. YAML. etc.). Provide for successful responses."`
	// Error declares the turn as failed with a human-readable reason.
	Error string `json:"error,omitempty" jsonschema:"description=Declare the turn as failed with a human-readable reason. Provide when you cannot complete the request and cannot recover."`
}

// TurnDoneResult is returned by turn_done. It implements coreloop.TurnDoner so the
// loop policy can detect turn completion without name-matching.
type TurnDoneResult struct {
	content string
	errMsg  string
}

// IsTurnDone satisfies core/loop.TurnDoner — signals the loop to stop cleanly.
func (TurnDoneResult) IsTurnDone() bool { return true }

// IsError satisfies tool.Result.
func (r TurnDoneResult) IsError() bool { return r.errMsg != "" }

// String satisfies fmt.Stringer — what the LLM sees in the tool result message.
func (r TurnDoneResult) String() string {
	switch {
	case r.errMsg != "" && r.content != "":
		return fmt.Sprintf("Turn failed: %s\n%s", r.errMsg, r.content)
	case r.errMsg != "":
		return fmt.Sprintf("Turn failed: %s", r.errMsg)
	case r.content != "":
		return r.content
	default:
		return "Turn done."
	}
}

// MarshalJSON satisfies tool.Result for history persistence.
func (r TurnDoneResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string `json:"type"`
		Content string `json:"content,omitempty"`
		Error   string `json:"error,omitempty"`
	}{Type: "turn_done", Content: r.content, Error: r.errMsg})
}

// Compile-time interface check.
var _ tool.Result = TurnDoneResult{}

// Tool returns the turn_done tool ready for registration.
func Tool() tool.Tool {
	return tool.New("turn_done",
		"Complete your turn. Provide rich content for successful responses, or set error to declare the turn as failed.",
		func(_ tool.Ctx, p TurnDoneParams) (tool.Result, error) {
			if p.Content == "" && p.Error == "" {
				return tool.Error("turn_done requires either 'content' or 'error'"), nil
			}
			return TurnDoneResult{content: p.Content, errMsg: p.Error}, nil
		},
	)
}

// Tools returns a slice containing the turn_done tool (convenience for WithTools).
func Tools() []tool.Tool { return []tool.Tool{Tool()} }
