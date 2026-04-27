package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
)

func TestProjectionKeepsPendingContextOutsideCompaction(t *testing.T) {
	tree := NewTree()
	old, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "old")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, CompactionEvent{Summary: "summary", Replaces: []NodeID{old}}); err != nil {
		t.Fatal(err)
	}
	items, err := ProjectItems(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}

	projection, err := DefaultProjectionPolicy().Project(ProjectionInput{
		Tree:   tree,
		Branch: MainBranch,
		Items:  items,
		PendingItems: []Item{{
			Kind: ItemContextFragment,
			Message: unified.Message{
				Role:    unified.RoleUser,
				Name:    "context",
				Content: []unified.ContentPart{unified.TextPart{Text: "latest context"}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(projection.Messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, projection.Messages)
	}
	requireTextPart(t, projection.Messages[0], "summary")
	requireTextPart(t, projection.Messages[1], "latest context")
}

func TestProjectionPlacesHistoryBeforePendingMessages(t *testing.T) {
	tree := NewTree()
	if _, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "history")}); err != nil {
		t.Fatal(err)
	}

	projection, err := DefaultProjectionPolicy().Project(ProjectionInput{
		Tree:         tree,
		Branch:       MainBranch,
		Items:        []Item{{Kind: ItemMessage, Message: textMessage(unified.RoleUser, "history")}},
		PendingItems: ItemsFromMessages([]unified.Message{textMessage(unified.RoleUser, "next")}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(projection.Messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, projection.Messages)
	}
	requireTextPart(t, projection.Messages[0], "history")
	requireTextPart(t, projection.Messages[1], "next")
}

func TestProjectionUsesNativeContinuationForPendingMessages(t *testing.T) {
	tree := NewTree()
	user, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "history")})
	if err != nil {
		t.Fatal(err)
	}
	assistant, err := tree.Append(MainBranch, AssistantTurnEvent{
		Message: textMessage(unified.RoleAssistant, "answer"),
		Continuations: []ProviderContinuation{{
			ProviderName:         "openai",
			APIKind:              "openai.responses",
			NativeModel:          "gpt-test",
			ResponseID:           "resp_1",
			ConsumerContinuation: unified.ContinuationPreviousResponseID,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, CompactionEvent{Summary: "summary", Replaces: []NodeID{user, assistant}}); err != nil {
		t.Fatal(err)
	}

	projection, err := DefaultProjectionPolicy().Project(ProjectionInput{
		Tree:                    tree,
		Branch:                  MainBranch,
		ProviderIdentity:        ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		PendingItems:            ItemsFromMessages([]unified.Message{textMessage(unified.RoleUser, "again")}),
		AllowNativeContinuation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(projection.Messages), 1; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, projection.Messages)
	}
	requireTextPart(t, projection.Messages[0], "again")
}

func TestProjectionDoesNotUseNativeContinuationAfterCompactionHead(t *testing.T) {
	tree := NewTree()
	user, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "old")})
	if err != nil {
		t.Fatal(err)
	}
	assistant, err := tree.Append(MainBranch, AssistantTurnEvent{
		Message: textMessage(unified.RoleAssistant, "answer"),
		Continuations: []ProviderContinuation{{
			ProviderName:         "openai",
			APIKind:              "openai.responses",
			NativeModel:          "gpt-test",
			ResponseID:           "resp_1",
			ConsumerContinuation: unified.ContinuationPreviousResponseID,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, CompactionEvent{Summary: "summary", Replaces: []NodeID{user, assistant}}); err != nil {
		t.Fatal(err)
	}
	items, err := ProjectItems(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}

	projection, err := DefaultProjectionPolicy().Project(ProjectionInput{
		Tree:                    tree,
		Branch:                  MainBranch,
		ProviderIdentity:        ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		Items:                   items,
		PendingItems:            ItemsFromMessages([]unified.Message{textMessage(unified.RoleUser, "again")}),
		AllowNativeContinuation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Extensions.Has(unified.ExtOpenAIPreviousResponseID) {
		t.Fatal("native continuation should not be used after compaction becomes branch head")
	}
	if got, want := len(projection.Messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, projection.Messages)
	}
	requireTextPart(t, projection.Messages[0], "summary")
	requireTextPart(t, projection.Messages[1], "again")
}
