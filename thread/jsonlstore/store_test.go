package jsonlstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/codewandler/agentsdk/thread"
)

func TestStorePersistsCreateAppendResumeFork(t *testing.T) {
	ctx := context.Background()
	store := Open(t.TempDir())
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, thread.Event{Kind: "capability.attached", Payload: json.RawMessage(`{"instance_id":"planner_1"}`)}); err != nil {
		t.Fatal(err)
	}
	alt, err := store.Fork(ctx, thread.ForkParams{ID: live.ID(), FromBranchID: thread.MainBranch, ToBranchID: "alt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := alt.Append(ctx, thread.Event{Kind: "capability.state_event_dispatched", Payload: json.RawMessage(`{"branch":"alt"}`)}); err != nil {
		t.Fatal(err)
	}

	reopened := Open(store.dir)
	resumed, err := reopened.Resume(ctx, thread.ResumeParams{ID: live.ID(), BranchID: "alt"})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.BranchID() != "alt" {
		t.Fatalf("branch = %q, want alt", resumed.BranchID())
	}
	stored, err := reopened.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := stored.EventsForBranch("alt")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(events), 4; got != want {
		t.Fatalf("alt events = %d, want %d", got, want)
	}
}

func TestLiveAppendFlushShutdownAndDiscard(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := Open(dir)
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_live"})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, thread.Event{Kind: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := live.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := live.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, thread.Event{Kind: "after_shutdown"}); err == nil {
		t.Fatal("expected append after shutdown to fail")
	}

	reopened := Open(dir)
	stored, err := reopened.Read(ctx, thread.ReadParams{ID: "thread_live"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stored.Events), 2; got != want {
		t.Fatalf("events = %d, want %d", got, want)
	}

	resumed, err := reopened.Resume(ctx, thread.ResumeParams{ID: "thread_live"})
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Discard(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Read(ctx, thread.ReadParams{ID: "thread_live"}); err == nil {
		t.Fatal("expected discarded thread read to fail")
	}
}

func TestStoreValidatesRegisteredEventsOnLoad(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	event := thread.Event{
		ID:       "evt_invalid",
		ThreadID: "thread_invalid",
		BranchID: thread.MainBranch,
		Seq:      1,
		Kind:     thread.EventBranchCreated,
		Payload:  json.RawMessage(`[]`),
	}
	raw, err := json.Marshal(encode(event))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "thread_invalid.jsonl"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := thread.NewEventRegistry(thread.CoreEventDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	store := Open(dir, WithEventRegistry(registry))
	if _, err := store.Read(ctx, thread.ReadParams{ID: "thread_invalid"}); err == nil {
		t.Fatal("expected load validation error")
	}
}

func TestStorePersistsEventSchemaVersion(t *testing.T) {
	ctx := context.Background()
	store := Open(t.TempDir())
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_schema_version"})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, thread.Event{Kind: "test.event"}); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range stored.Events {
		if event.SchemaVersion != thread.CurrentEventSchemaVersion {
			t.Fatalf("schema version = %d, want %d", event.SchemaVersion, thread.CurrentEventSchemaVersion)
		}
	}
}
