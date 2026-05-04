package datasourceexample

import (
	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/datasource"
)

// DocumentationCorpus returns a datasource definition for a documentation corpus.
// The datasource names schemas, provenance, checkpointing, and standard action
// refs, but it does not execute work itself.
func DocumentationCorpus() datasource.Definition {
	return datasource.Definition{
		Name:        "docs",
		Description: "Local documentation corpus indexed for examples",
		Kind:        datasource.KindCorpus,
		ConfigSchema: datasource.SchemaRef{Inline: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"root": map[string]any{"type": "string"},
			},
		}},
		RecordSchema: datasource.SchemaRef{Inline: map[string]any{
			"type":     "object",
			"required": []any{"path", "text"},
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
				"text": map[string]any{"type": "string"},
			},
		}},
		Provenance: datasource.Provenance{Source: "filesystem", URI: "file://docs"},
		Checkpoint: datasource.CheckpointSpec{StateKey: "docs.cursor"},
		Freshness:  datasource.FreshnessSpec{TTL: "24h", Consistency: "eventual"},
		Actions: datasource.Actions{
			List:   action.Ref{Name: "docs.list"},
			Search: action.Ref{Name: "docs.search"},
			Sync:   action.Ref{Name: "docs.sync"},
		},
	}
}

func Registry() (*datasource.Registry, error) {
	reg := datasource.NewRegistry()
	if err := reg.Register(DocumentationCorpus()); err != nil {
		return nil, err
	}
	return reg, nil
}
