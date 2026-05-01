package datasource

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryRegistersAndListsDatasources(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(
		Definition{Name: "docs", Kind: KindCorpus},
		Definition{Name: "tickets", Kind: KindAPI},
	))

	def, ok := reg.Get("docs")
	require.True(t, ok)
	require.Equal(t, KindCorpus, def.Kind)
	require.Equal(t, []Definition{{Name: "docs", Kind: KindCorpus}, {Name: "tickets", Kind: KindAPI}}, reg.All())
}

func TestRegistryRejectsInvalidAndDuplicateDatasources(t *testing.T) {
	reg := NewRegistry()
	require.ErrorContains(t, reg.Register(Definition{Name: "missing-kind"}), "kind is required")

	require.NoError(t, reg.Register(Definition{Name: "docs", Kind: KindCorpus}))
	err := reg.Register(Definition{Name: "docs", Kind: KindAPI})
	var dup ErrDuplicate
	require.True(t, errors.As(err, &dup))
	require.Equal(t, "docs", dup.Name)
}
