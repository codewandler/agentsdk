package skill

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefMetadata_AllTriggers_ListForm(t *testing.T) {
	r := &RefMetadata{Triggers: []string{"make a plan", "write a plan"}}
	got := r.AllTriggers()
	require.Equal(t, []string{"make a plan", "write a plan"}, got)
}

func TestRefMetadata_AllTriggers_StringForm(t *testing.T) {
	r := &RefMetadata{Trigger: "make a plan, write a plan, break down"}
	got := r.AllTriggers()
	require.Equal(t, []string{"make a plan", "write a plan", "break down"}, got)
}

func TestRefMetadata_AllTriggers_Empty(t *testing.T) {
	r := &RefMetadata{}
	got := r.AllTriggers()
	require.Nil(t, got)
}

func TestRefMetadata_AllTriggers_ListPrecedence(t *testing.T) {
	// When Triggers list is present, Trigger string is ignored
	r := &RefMetadata{
		Triggers: []string{"foo", "bar"},
		Trigger:  "baz, qux",
	}
	got := r.AllTriggers()
	require.Equal(t, []string{"foo", "bar"}, got)
}

func TestRefMetadata_AllTriggers_TrimSpace(t *testing.T) {
	r := &RefMetadata{Trigger: "  make a plan  ,  write a plan  "}
	got := r.AllTriggers()
	require.Equal(t, []string{"make a plan", "write a plan"}, got)
}

func TestRefMetadata_AllTriggers_DedupeString(t *testing.T) {
	// Duplicate triggers in string form are deduplicated
	r := &RefMetadata{Trigger: "make a plan, make a plan, write a plan"}
	got := r.AllTriggers()
	require.Equal(t, []string{"make a plan", "write a plan"}, got)
}

func TestRefMetadata_When_IsNilByDefault(t *testing.T) {
	r := &RefMetadata{}
	require.Nil(t, r.When)
}

func TestRefMetadata_When_CanBeSet(t *testing.T) {
	r := &RefMetadata{
		When: &WhenEntry{Language: "golang"},
	}
	require.NotNil(t, r.When)
	require.Equal(t, "golang", r.When.Language)
}
