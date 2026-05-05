package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testIndex() *ResourceIndex {
	idx := NewResourceIndex()
	idx.Add(ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"})
	idx.Add(ResourceID{Kind: "command", Origin: "local", Namespace: NewNamespace("my-app"), Name: "commit"})
	idx.Add(ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "review"})
	idx.Add(ResourceID{Kind: "agent", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "main"})
	idx.Add(ResourceID{Kind: "agent", Origin: "local", Namespace: NewNamespace("my-app"), Name: "main"})
	idx.Add(ResourceID{Kind: "command", Origin: "user", Namespace: NewNamespace("global"), Name: "scratch"})
	return idx
}

func TestResolverUniqueName(t *testing.T) {
	r := NewResolver(ResolverConfig{Index: testIndex()})

	// "review" is unique — resolves directly.
	id, err := r.Resolve("command", "review")
	require.NoError(t, err)
	require.Equal(t, "agentsdk:engineer:review", id.Address())

	// "scratch" is unique.
	id, err = r.Resolve("command", "scratch")
	require.NoError(t, err)
	require.Equal(t, "user:global:scratch", id.Address())
}

func TestResolverFullyQualified(t *testing.T) {
	r := NewResolver(ResolverConfig{Index: testIndex()})

	id, err := r.Resolve("command", "agentsdk:engineer:commit")
	require.NoError(t, err)
	require.Equal(t, "agentsdk", id.Origin)

	id, err = r.Resolve("command", "local:commit")
	require.NoError(t, err)
	require.Equal(t, "local", id.Origin)
}

func TestResolverPrecedencePolicy(t *testing.T) {
	r := NewResolver(ResolverConfig{
		Index:  testIndex(),
		Policy: PrecedencePolicy{}, // default order: explicit, local, user, embedded
	})

	// "commit" is ambiguous: agentsdk (not in default order → unranked) vs local.
	// local has higher precedence than unranked.
	id, err := r.Resolve("command", "commit")
	require.NoError(t, err)
	require.Equal(t, "local", id.Origin)
}

func TestResolverPrecedencePolicyCustomOrder(t *testing.T) {
	r := NewResolver(ResolverConfig{
		Index:  testIndex(),
		Policy: PrecedencePolicy{Order: []string{"agentsdk", "local", "user"}},
	})

	// With agentsdk first, it wins.
	id, err := r.Resolve("command", "commit")
	require.NoError(t, err)
	require.Equal(t, "agentsdk", id.Origin)
}

func TestResolverPrecedencePolicyAgentKind(t *testing.T) {
	r := NewResolver(ResolverConfig{
		Index:  testIndex(),
		Policy: PrecedencePolicy{},
	})

	// "main" agent: local > agentsdk (unranked).
	id, err := r.Resolve("agent", "main")
	require.NoError(t, err)
	require.Equal(t, "local", id.Origin)
}

func TestResolverErrorPolicy(t *testing.T) {
	r := NewResolver(ResolverConfig{
		Index:  testIndex(),
		Policy: ErrorPolicy{},
	})

	_, err := r.Resolve("command", "commit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
	require.Contains(t, err.Error(), "agentsdk:engineer:commit")
	require.Contains(t, err.Error(), "local:my-app:commit")
}

func TestResolverNotFound(t *testing.T) {
	r := NewResolver(ResolverConfig{Index: testIndex()})

	_, err := r.Resolve("command", "nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no command found")
}

func TestResolverAlias(t *testing.T) {
	r := NewResolver(ResolverConfig{
		Index:   testIndex(),
		Policy:  ErrorPolicy{}, // would error on ambiguity without alias
		Aliases: map[string]string{"commit": "agentsdk:engineer:commit"},
	})

	id, err := r.Resolve("command", "commit")
	require.NoError(t, err)
	require.Equal(t, "agentsdk:engineer:commit", id.Address())
}

func TestResolverSetAlias(t *testing.T) {
	r := NewResolver(ResolverConfig{
		Index:  testIndex(),
		Policy: ErrorPolicy{},
	})

	// Without alias, ambiguous.
	_, err := r.Resolve("command", "commit")
	require.Error(t, err)

	// Set alias, now resolves.
	r.SetAlias("commit", "local:commit")
	id, err := r.Resolve("command", "commit")
	require.NoError(t, err)
	require.Equal(t, "local", id.Origin)
}

func TestResolverCachesResolution(t *testing.T) {
	r := NewResolver(ResolverConfig{Index: testIndex()})

	// First resolve.
	_, err := r.Resolve("command", "review")
	require.NoError(t, err)

	// Check cache.
	resolved := r.Resolved()
	require.Contains(t, resolved, "command:review")
	require.Equal(t, "agentsdk:engineer:review", resolved["command:review"])

	// Second resolve hits cache.
	id, err := r.Resolve("command", "review")
	require.NoError(t, err)
	require.Equal(t, "agentsdk:engineer:review", id.Address())
}

func TestResolverKindIsolation(t *testing.T) {
	r := NewResolver(ResolverConfig{Index: testIndex()})

	// "main" exists as agent but not as command.
	_, err := r.Resolve("command", "main")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no command found")

	id, err := r.Resolve("agent", "main")
	require.NoError(t, err)
	require.Equal(t, "main", id.Name)
}
