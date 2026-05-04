package datasourceexample

import (
	"testing"

	"github.com/codewandler/agentsdk/datasource"
	"github.com/stretchr/testify/require"
)

func TestDocumentationCorpus(t *testing.T) {
	def := DocumentationCorpus()
	require.NoError(t, datasource.Validate(def))
	require.Equal(t, "docs", def.Name)
	require.Equal(t, datasource.KindCorpus, def.Kind)
	require.Len(t, def.Actions.All(), 3)

	reg, err := Registry()
	require.NoError(t, err)
	stored, ok := reg.Get("docs")
	require.True(t, ok)
	require.Equal(t, def.Name, stored.Name)
}
