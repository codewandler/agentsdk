package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceIndexAddAndLookup(t *testing.T) {
	idx := NewResourceIndex()
	a := ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"}
	b := ResourceID{Kind: "command", Origin: "local", Namespace: NewNamespace("my-app"), Name: "commit"}
	c := ResourceID{Kind: "agent", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "main"}

	idx.Add(a)
	idx.Add(b)
	idx.Add(c)

	require.Equal(t, 3, idx.Len())

	// Lookup by name + kind.
	commands := idx.Lookup("command", "commit")
	require.Len(t, commands, 2)

	agents := idx.Lookup("agent", "main")
	require.Len(t, agents, 1)
	require.Equal(t, "main", agents[0].Name)

	// Lookup without kind filter.
	all := idx.Lookup("", "commit")
	require.Len(t, all, 2)

	// No match.
	require.Empty(t, idx.Lookup("command", "nonexistent"))
}

func TestResourceIndexDeduplicates(t *testing.T) {
	idx := NewResourceIndex()
	a := ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"}
	idx.Add(a)
	idx.Add(a) // duplicate
	idx.Add(a) // duplicate

	require.Equal(t, 1, idx.Len())
}

func TestResourceIndexLookupRef(t *testing.T) {
	idx := NewResourceIndex()
	a := ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"}
	b := ResourceID{Kind: "command", Origin: "local", Namespace: NewNamespace("my-app"), Name: "commit"}
	c := ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "review"}

	idx.Add(a)
	idx.Add(b)
	idx.Add(c)

	// Unqualified — both commits.
	results := idx.LookupRef("command", "commit")
	require.Len(t, results, 2)

	// Qualified by namespace.
	results = idx.LookupRef("command", "engineer:commit")
	require.Len(t, results, 1)
	require.Equal(t, "agentsdk", results[0].Origin)

	// Qualified by origin.
	results = idx.LookupRef("command", "local:commit")
	require.Len(t, results, 1)
	require.Equal(t, "local", results[0].Origin)

	// Fully qualified.
	results = idx.LookupRef("command", "agentsdk:engineer:commit")
	require.Len(t, results, 1)

	// No match.
	results = idx.LookupRef("command", "builder:commit")
	require.Empty(t, results)

	// Different kind filtered out.
	results = idx.LookupRef("agent", "commit")
	require.Empty(t, results)
}

func TestResourceIndexAll(t *testing.T) {
	idx := NewResourceIndex()
	idx.Add(ResourceID{Kind: "command", Origin: "a", Name: "x"})
	idx.Add(ResourceID{Kind: "agent", Origin: "b", Name: "y"})
	idx.Add(ResourceID{Kind: "skill", Origin: "c", Name: "z"})

	all := idx.All()
	require.Len(t, all, 3)
}
