package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamespaceString(t *testing.T) {
	tests := []struct {
		segments []string
		want     string
	}{
		{nil, ""},
		{[]string{"engineer"}, "engineer"},
		{[]string{"user", "repo", "plugins", "foo"}, "user/repo/plugins/foo"},
		{[]string{" a ", "", "b"}, "a/b"}, // trims and skips empty
	}
	for _, tt := range tests {
		ns := NewNamespace(tt.segments...)
		require.Equal(t, tt.want, ns.String())
	}
}

func TestNamespaceLast(t *testing.T) {
	require.Equal(t, "", NewNamespace().Last())
	require.Equal(t, "foo", NewNamespace("a", "b", "foo").Last())
}

func TestNamespaceEqual(t *testing.T) {
	a := NewNamespace("a", "b")
	b := NewNamespace("a", "b")
	c := NewNamespace("a", "c")
	require.True(t, a.Equal(b))
	require.False(t, a.Equal(c))
	require.True(t, NewNamespace().Equal(NewNamespace()))
}

func TestNamespaceSuffixMatch(t *testing.T) {
	ns := NewNamespace("a", "b", "c")
	require.True(t, ns.SuffixMatch([]string{"c"}))
	require.True(t, ns.SuffixMatch([]string{"b", "c"}))
	require.True(t, ns.SuffixMatch([]string{"a", "b", "c"}))
	require.False(t, ns.SuffixMatch([]string{"a", "c"}))
	require.False(t, ns.SuffixMatch([]string{"x"}))
	require.False(t, ns.SuffixMatch([]string{"a", "b", "c", "d"}))
	require.True(t, ns.SuffixMatch(nil))
	require.True(t, ns.SuffixMatch([]string{}))
}

func TestNamespaceAppend(t *testing.T) {
	ns := NewNamespace("a", "b")
	extended := ns.Append("c", "d")
	require.Equal(t, "a/b", ns.String())       // original unchanged
	require.Equal(t, "a/b/c/d", extended.String())
}

func TestResourceIDAddress(t *testing.T) {
	tests := []struct {
		id   ResourceID
		want string
	}{
		{
			ResourceID{Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"},
			"agentsdk:engineer:commit",
		},
		{
			ResourceID{Origin: "local", Namespace: NewNamespace("my-app"), Name: "deploy"},
			"local:my-app:deploy",
		},
		{
			ResourceID{Origin: "github.com", Namespace: NewNamespace("user", "repo", "plugins", "foo"), Name: "fruit"},
			"github.com:user/repo/plugins/foo:fruit",
		},
		{
			ResourceID{Origin: "user", Namespace: NewNamespace(), Name: "scratch"},
			"user:scratch",
		},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.id.Address())
		require.Equal(t, tt.want, tt.id.String())
	}
}

func TestResourceIDIsZero(t *testing.T) {
	require.True(t, ResourceID{}.IsZero())
	require.False(t, ResourceID{Origin: "local"}.IsZero())
	require.False(t, ResourceID{Name: "x"}.IsZero())
}

func TestResourceIDEqual(t *testing.T) {
	a := ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"}
	b := ResourceID{Kind: "command", Origin: "agentsdk", Namespace: NewNamespace("engineer"), Name: "commit"}
	c := ResourceID{Kind: "command", Origin: "local", Namespace: NewNamespace("engineer"), Name: "commit"}
	require.True(t, a.Equal(b))
	require.False(t, a.Equal(c))
}

func TestResourceIDMatchesRef(t *testing.T) {
	id := ResourceID{
		Kind:      "command",
		Origin:    "agentsdk",
		Namespace: NewNamespace("engineer"),
		Name:      "commit",
	}

	// Name only.
	require.True(t, id.MatchesRef("commit"))

	// Namespace suffix + name.
	require.True(t, id.MatchesRef("engineer:commit"))

	// Fully qualified.
	require.True(t, id.MatchesRef("agentsdk:engineer:commit"))

	// Origin + name (namespace skipped).
	require.True(t, id.MatchesRef("agentsdk:commit"))

	// Wrong origin.
	require.False(t, id.MatchesRef("local:commit"))
	require.False(t, id.MatchesRef("local:engineer:commit"))

	// Wrong name.
	require.False(t, id.MatchesRef("review"))

	// Wrong namespace.
	require.False(t, id.MatchesRef("builder:commit"))

	// Empty ref.
	require.False(t, id.MatchesRef(""))
}

func TestResourceIDMatchesRefDeepNamespace(t *testing.T) {
	id := ResourceID{
		Kind:      "command",
		Origin:    "github.com",
		Namespace: NewNamespace("user", "repo", "plugins", "foo"),
		Name:      "fruit",
	}

	require.True(t, id.MatchesRef("fruit"))
	require.True(t, id.MatchesRef("foo:fruit"))
	require.True(t, id.MatchesRef("plugins:foo:fruit"))
	require.True(t, id.MatchesRef("repo:plugins:foo:fruit"))
	require.True(t, id.MatchesRef("user:repo:plugins:foo:fruit"))
	require.True(t, id.MatchesRef("github.com:user:repo:plugins:foo:fruit"))

	require.False(t, id.MatchesRef("bar:fruit"))
	require.False(t, id.MatchesRef("local:foo:fruit"))
}

func TestDeriveOrigin(t *testing.T) {
	tests := []struct {
		scope Scope
		want  string
	}{
		{ScopeEmbedded, "embedded"},
		{ScopeProject, "local"},
		{ScopeUser, "user"},
		{ScopeRemote, "remote"},
		{ScopeGit, "git"},
	}
	for _, tt := range tests {
		got := DeriveOrigin(SourceRef{Scope: tt.scope})
		require.Equal(t, tt.want, got, "scope=%s", tt.scope)
	}
}

func TestDeriveNamespace(t *testing.T) {
	tests := []struct {
		name   string
		source SourceRef
		want   string
	}{
		{"user scope", SourceRef{Scope: ScopeUser, Root: "/home/user/.agents"}, "global"},
		{"project scope parent dir", SourceRef{Scope: ScopeProject, Root: "/home/user/projects/my-app/.agents"}, "my-app"},
		{"embedded scope", SourceRef{Scope: ScopeEmbedded, Root: "resources"}, "resources"},
		{"embedded dot-agents", SourceRef{Scope: ScopeEmbedded, Root: ".agents"}, "agents"},
		{"remote scope", SourceRef{Scope: ScopeRemote, Root: "github.com/acme/tools"}, "github.com/acme/tools"},
	}
	for _, tt := range tests {
		got := DeriveNamespace(tt.source)
		require.Equal(t, tt.want, got.String(), tt.name)
	}
}

func TestDeriveResourceID(t *testing.T) {
	source := SourceRef{Scope: ScopeProject, Root: "/home/user/my-app/.agents", Ecosystem: "agents"}
	id := DeriveResourceID(source, "command", "commit")
	require.Equal(t, "command", id.Kind)
	require.Equal(t, "local", id.Origin)
	require.Equal(t, "commit", id.Name)
	require.Equal(t, "local:my-app:commit", id.Address())
}
