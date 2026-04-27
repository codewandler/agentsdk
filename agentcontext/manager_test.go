package agentcontext

import (
	"context"
	"testing"

	"github.com/codewandler/llmadapter/unified"
)

func TestManagerBuildAddsUpdatesAndRemovesFragments(t *testing.T) {
	provider := &staticProvider{
		key: "planner",
		fragments: []ContextFragment{
			{Key: "planner/meta", Content: "2 steps", Role: unified.RoleUser},
			{Key: "planner/step/a", Content: "pending: inspect", Role: unified.RoleUser},
			{Key: "planner/step/b", Content: "pending: implement", Role: unified.RoleUser},
		},
	}
	manager, err := NewManager(provider)
	if err != nil {
		t.Fatal(err)
	}

	first, err := manager.Build(context.Background(), BuildRequest{ThreadID: "thread_1", BranchID: "main", Preference: PreferFull})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(first.Added), 3; got != want {
		t.Fatalf("first added = %d, want %d", got, want)
	}
	if got, want := len(first.Active), 3; got != want {
		t.Fatalf("first active = %d, want %d", got, want)
	}

	second, err := manager.Build(context.Background(), BuildRequest{ThreadID: "thread_1", BranchID: "main", Preference: PreferChanges})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Added) != 0 || len(second.Updated) != 0 || len(second.Removed) != 0 {
		t.Fatalf("second diff = added %d updated %d removed %d, want no-op", len(second.Added), len(second.Updated), len(second.Removed))
	}

	provider.fragments = []ContextFragment{
		{Key: "planner/meta", Content: "1 step", Role: unified.RoleUser},
		{Key: "planner/step/a", Content: "completed: inspect", Role: unified.RoleUser},
	}
	third, err := manager.Build(context.Background(), BuildRequest{ThreadID: "thread_1", BranchID: "main", Preference: PreferChanges})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(third.Updated), []FragmentKey{"planner/meta", "planner/step/a"}; !sameKeys(got, want) {
		t.Fatalf("updated keys = %v, want %v", got, want)
	}
	if got, want := len(third.Removed), 1; got != want {
		t.Fatalf("removed = %d, want %d", got, want)
	}
	if third.Removed[0].FragmentKey != "planner/step/b" {
		t.Fatalf("removed key = %q, want planner/step/b", third.Removed[0].FragmentKey)
	}
	if got, want := len(third.Active), 2; got != want {
		t.Fatalf("third active = %d, want %d", got, want)
	}

	fourth, err := manager.Build(context.Background(), BuildRequest{ThreadID: "thread_1", BranchID: "main", Preference: PreferChanges})
	if err != nil {
		t.Fatal(err)
	}
	if len(fourth.Removed) != 0 {
		t.Fatalf("fourth removed = %d, want no repeated tombstone", len(fourth.Removed))
	}
}

func TestManagerRejectsDuplicateProvidersAndFragments(t *testing.T) {
	if _, err := NewManager(&staticProvider{key: "dup"}, &staticProvider{key: "dup"}); err == nil {
		t.Fatal("expected duplicate provider error")
	}

	manager, err := NewManager(&staticProvider{
		key: "dup-fragment",
		fragments: []ContextFragment{
			{Key: "same", Content: "a"},
			{Key: "same", Content: "b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Build(context.Background(), BuildRequest{}); err == nil {
		t.Fatal("expected duplicate fragment error")
	}
}

func TestManagerUsesFingerprintFastPath(t *testing.T) {
	provider := &staticProvider{
		key:         "env",
		fingerprint: "same",
		fragments:   []ContextFragment{{Key: "env/cwd", Content: "/repo"}},
	}
	manager, err := NewManager(provider)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Build(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Added) != 1 {
		t.Fatalf("first added = %d, want 1", len(first.Added))
	}

	provider.fragments = []ContextFragment{{Key: "env/cwd", Content: "/repo changed"}}
	second, err := manager.Build(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 because second build used fast path", provider.calls)
	}
	if len(second.Providers) != 1 || !second.Providers[0].Skipped {
		t.Fatalf("provider result skipped = %#v, want skipped", second.Providers)
	}
	if got := second.Active[0].Content; got != "/repo" {
		t.Fatalf("active content = %q, want cached /repo", got)
	}
}

func TestManagerPrepareCommitAndRollback(t *testing.T) {
	provider := &staticProvider{
		key:       "env",
		fragments: []ContextFragment{{Key: "env/cwd", Content: "/repo"}},
	}
	manager, err := NewManager(provider)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := manager.Prepare(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Result.Added) != 1 {
		t.Fatalf("prepared added = %d, want 1", len(prepared.Result.Added))
	}
	prepared.Rollback()

	again, err := manager.Prepare(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Result.Added) != 1 {
		t.Fatalf("again added = %d, want 1 after rollback", len(again.Result.Added))
	}
	if err := again.Commit(); err != nil {
		t.Fatal(err)
	}

	noChange, err := manager.Prepare(context.Background(), BuildRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(noChange.Result.Added) != 0 || len(noChange.Result.Updated) != 0 || len(noChange.Result.Removed) != 0 {
		t.Fatalf("noChange diff = added %d updated %d removed %d, want no-op", len(noChange.Result.Added), len(noChange.Result.Updated), len(noChange.Result.Removed))
	}
}

type staticProvider struct {
	key         ProviderKey
	fragments   []ContextFragment
	fingerprint string
	calls       int
}

func (p *staticProvider) Key() ProviderKey { return p.key }

func (p *staticProvider) GetContext(context.Context, Request) (ProviderContext, error) {
	p.calls++
	return ProviderContext{Fragments: append([]ContextFragment(nil), p.fragments...), Fingerprint: p.fingerprint}, nil
}

func (p *staticProvider) StateFingerprint(context.Context, Request) (string, bool, error) {
	if p.fingerprint == "" {
		return "", false, nil
	}
	return p.fingerprint, true, nil
}

func keys(fragments []ContextFragment) []FragmentKey {
	out := make([]FragmentKey, len(fragments))
	for i, fragment := range fragments {
		out[i] = fragment.Key
	}
	return out
}

func sameKeys(a, b []FragmentKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
