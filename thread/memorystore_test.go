package thread

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryStoreCreateAppendReadAndAtomicValidation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	live, err := store.Create(ctx, CreateParams{
		ID:       "thread_test",
		BranchID: MainBranch,
		Metadata: map[string]string{"title": "test"},
		Source:   EventSource{Type: "test", ID: "runner"},
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := live.Append(ctx,
		Event{Kind: "conversation.user_message", Payload: json.RawMessage(`{"text":"hi"}`)},
		Event{Kind: "conversation.assistant_message", Payload: json.RawMessage(`{"text":"hello"}`)},
	); err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, Event{Kind: "conversation.user_message"}, Event{}); err == nil {
		t.Fatal("expected invalid batch error")
	}

	stored, err := store.Read(ctx, ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stored.Events), 3; got != want {
		t.Fatalf("events = %d, want %d", got, want)
	}
	for i, event := range stored.Events {
		if got, want := event.Seq, int64(i+1); got != want {
			t.Fatalf("seq[%d] = %d, want %d", i, got, want)
		}
		if event.ThreadID != "thread_test" {
			t.Fatalf("thread id = %q", event.ThreadID)
		}
		if event.BranchID != MainBranch {
			t.Fatalf("branch id = %q", event.BranchID)
		}
	}
	if stored.Events[1].Source.Type != "test" || stored.Events[1].Source.ID != "runner" {
		t.Fatalf("default source not applied: %#v", stored.Events[1].Source)
	}
}

func TestMemoryStoreReadReturnsImmutableCopies(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	live, err := store.Create(ctx, CreateParams{ID: "thread_copy", Metadata: map[string]string{"a": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, Event{Kind: "conversation.user_message", Payload: json.RawMessage(`{"text":"hi"}`)}); err != nil {
		t.Fatal(err)
	}

	first, err := store.Read(ctx, ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	first.Metadata["a"] = "changed"
	first.Events[1].Payload[9] = 'X'

	second, err := store.Read(ctx, ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if second.Metadata["a"] != "b" {
		t.Fatalf("metadata mutated through read copy: %q", second.Metadata["a"])
	}
	if string(second.Events[1].Payload) != `{"text":"hi"}` {
		t.Fatalf("payload mutated through read copy: %s", second.Events[1].Payload)
	}
}

func TestMemoryStoreArchiveListResumeAndDiscard(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	live, err := store.Create(ctx, CreateParams{ID: "thread_archive"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(ctx, live.ID()); err != nil {
		t.Fatal(err)
	}
	page, err := store.List(ctx, ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Threads) != 0 {
		t.Fatalf("listed archived thread without IncludeArchived")
	}
	page, err = store.List(ctx, ListParams{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(page.Threads), 1; got != want {
		t.Fatalf("archived list = %d, want %d", got, want)
	}
	resumed, err := store.Resume(ctx, ResumeParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Discard(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(ctx, ReadParams{ID: live.ID()}); err == nil {
		t.Fatal("expected discarded thread to be removed")
	}
}
