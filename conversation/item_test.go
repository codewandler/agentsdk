package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
)

func TestProjectItemsNormalizesMessageAndAssistantTurns(t *testing.T) {
	tree := NewTree()
	if _, err := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "hi"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, AssistantTurnEvent{Message: unified.Message{Role: unified.RoleAssistant, Content: []unified.ContentPart{unified.TextPart{Text: "hello"}}}, FinishReason: unified.FinishReasonStop}); err != nil {
		t.Fatal(err)
	}

	items, err := ProjectItems(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(items), 2; got != want {
		t.Fatalf("items = %d, want %d", got, want)
	}
	if items[0].Kind != ItemMessage || items[1].Kind != ItemAssistantTurn {
		t.Fatalf("item kinds = %q %q", items[0].Kind, items[1].Kind)
	}
	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d", got, want)
	}
}

func TestNormalizeItemsInsertsMissingToolResults(t *testing.T) {
	items := []Item{{
		Kind: ItemAssistantTurn,
		Message: unified.Message{
			Role: unified.RoleAssistant,
			ToolCalls: []unified.ToolCall{{
				ID:   "call_1",
				Name: "plan",
			}},
		},
	}}

	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d", got, want)
	}
	if messages[1].Role != unified.RoleTool || len(messages[1].ToolResults) != 1 {
		t.Fatalf("missing synthetic tool result: %#v", messages[1])
	}
	result := messages[1].ToolResults[0]
	if result.ToolCallID != "call_1" || !result.IsError {
		t.Fatalf("synthetic result = %#v", result)
	}
}

func TestNormalizeItemsDropsOrphanToolResults(t *testing.T) {
	items := []Item{{
		Kind: ItemMessage,
		Message: unified.Message{
			Role: unified.RoleTool,
			ToolResults: []unified.ToolResult{{
				ToolCallID: "orphan",
				Name:       "plan",
				Content:    []unified.ContentPart{unified.TextPart{Text: "ignored"}},
			}},
		},
	}}

	messages := MessagesFromItems(items)
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want orphan tool result dropped", messages)
	}
}

func TestNormalizeItemsDropsDuplicateToolCallsAndResults(t *testing.T) {
	items := []Item{
		{
			Kind: ItemAssistantTurn,
			Message: unified.Message{
				Role: unified.RoleAssistant,
				ToolCalls: []unified.ToolCall{
					{ID: "call_1", Name: "plan"},
					{ID: "call_1", Name: "plan_duplicate"},
				},
			},
		},
		{
			Kind: ItemMessage,
			Message: unified.Message{
				Role: unified.RoleTool,
				ToolResults: []unified.ToolResult{
					{ToolCallID: "call_1", Name: "plan", Content: []unified.ContentPart{unified.TextPart{Text: "ok"}}},
					{ToolCallID: "call_1", Name: "plan", Content: []unified.ContentPart{unified.TextPart{Text: "duplicate"}}},
				},
			},
		},
	}

	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, messages)
	}
	if got, want := len(messages[0].ToolCalls), 1; got != want {
		t.Fatalf("tool calls = %d, want %d", got, want)
	}
	if got, want := messages[0].ToolCalls[0].Name, "plan"; got != want {
		t.Fatalf("tool call name = %q, want %q", got, want)
	}
	if got, want := len(messages[1].ToolResults), 1; got != want {
		t.Fatalf("tool results = %d, want %d", got, want)
	}
	text, ok := messages[1].ToolResults[0].Content[0].(unified.TextPart)
	if !ok || text.Text != "ok" {
		t.Fatalf("tool result content = %#v", messages[1].ToolResults[0].Content)
	}
}

func TestNormalizeItemsStripsUnsupportedMedia(t *testing.T) {
	items := []Item{{
		Kind: ItemMessage,
		Message: unified.Message{
			Role: unified.RoleUser,
			Content: []unified.ContentPart{
				unified.TextPart{Text: "keep"},
				unified.ImagePart{Alt: "empty"},
				unified.FilePart{Filename: "empty.txt"},
			},
		},
	}}

	messages := MessagesFromItems(items)
	if got, want := len(messages[0].Content), 1; got != want {
		t.Fatalf("content parts = %d, want %d: %#v", got, want, messages[0].Content)
	}
}

func TestExpandItemsDerivesToolReasoningAndContinuationItems(t *testing.T) {
	assistant := AssistantTurnEvent{
		Message: unified.Message{
			Role: unified.RoleAssistant,
			Content: []unified.ContentPart{
				unified.ReasoningPart{Text: "think", Signature: "sig"},
				unified.TextPart{Text: "done"},
			},
			ToolCalls: []unified.ToolCall{{ID: "call_1", Name: "plan"}},
		},
		Continuations: []ProviderContinuation{{ResponseID: "resp_1"}},
	}
	items := ExpandItems([]Item{{Kind: ItemAssistantTurn, Message: assistant.Message, Assistant: &assistant}})
	var sawReasoning, sawToolCall, sawContinuation bool
	for _, item := range items {
		switch item.Kind {
		case ItemReasoning:
			sawReasoning = item.Reasoning.Text == "think"
		case ItemToolCall:
			sawToolCall = item.ToolCall.ID == "call_1"
		case ItemContinuation:
			sawContinuation = item.Continuation.ResponseID == "resp_1"
		}
	}
	if !sawReasoning || !sawToolCall || !sawContinuation {
		t.Fatalf("derived items missing: reasoning=%v toolCall=%v continuation=%v items=%#v", sawReasoning, sawToolCall, sawContinuation, items)
	}
}
