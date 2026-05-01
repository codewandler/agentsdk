package datasource

import (
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

func TestValidateAcceptsDatasourceDefinition(t *testing.T) {
	def := Definition{
		Name:        "docs",
		Description: "documentation corpus",
		Kind:        KindCorpus,
		ConfigSchema: SchemaRef{Inline: map[string]any{
			"type": "object",
		}},
		RecordSchema: SchemaRef{URI: "schema://docs-record"},
		Provenance:   Provenance{Source: "docs-api", URI: "https://example.test/docs"},
		Credentials:  []CredentialRef{{Name: "docs_api_token", Kind: "bearer"}},
		Checkpoint:   CheckpointSpec{CursorField: "updated_at", StateKey: "docs.cursor"},
		Freshness:    FreshnessSpec{TTL: "24h", Consistency: "eventual"},
		Actions:      Actions{Search: action.Ref{Name: "docs.search"}, Sync: action.Ref{Name: "docs.sync"}},
	}

	require.NoError(t, Validate(def))
	require.Equal(t, []action.Ref{{Name: "docs.search"}, {Name: "docs.sync"}}, def.Actions.All())
}

func TestValidateRejectsInvalidDatasourceDefinitions(t *testing.T) {
	require.ErrorContains(t, Validate(Definition{Kind: KindCorpus}), "name is required")
	require.ErrorContains(t, Validate(Definition{Name: "docs"}), "kind is required")
	require.ErrorContains(t, Validate(Definition{Name: "docs", Kind: KindCorpus, Credentials: []CredentialRef{{}}}), "credential name")
	require.ErrorContains(t, Validate(Definition{Name: "docs", Kind: KindCorpus, Credentials: []CredentialRef{{Name: "token"}, {Name: "token"}}}), "duplicate credential")
	require.ErrorContains(t, Validate(Definition{Name: "docs", Kind: KindCorpus, Actions: Actions{Search: action.Ref{Name: "docs"}}}), "must not point at datasource name")
}
