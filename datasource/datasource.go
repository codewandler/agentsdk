// Package datasource defines configured data boundaries used by actions,
// workflows, tools, commands, triggers, and app code.
package datasource

import "github.com/codewandler/agentsdk/action"

// Kind classifies the external or internal data boundary represented by a
// datasource definition. Kinds are intentionally descriptive rather than an
// exhaustive execution enum; behavior is supplied by actions.
type Kind string

const (
	KindCorpus Kind = "corpus"
	KindAPI    Kind = "api"
	KindIndex  Kind = "index"
	KindStream Kind = "stream"
	KindDB     Kind = "db"
	KindFile   Kind = "file"
)

// Definition describes a configured data boundary. A datasource is not an
// execution primitive and is not workflow-owned. Actions perform work against
// datasources; those actions can then be exposed through workflows, tools,
// commands, triggers, or direct app code.
type Definition struct {
	Name        string
	Description string
	Kind        Kind

	ConfigSchema SchemaRef
	RecordSchema SchemaRef

	Provenance  Provenance
	Credentials []CredentialRef
	Checkpoint  CheckpointSpec
	Freshness   FreshnessSpec

	Actions  Actions
	Metadata map[string]any
}

// SchemaRef describes a schema associated with datasource config or records.
// The schema may be inline, referenced by URI, or both depending on the surface
// that produced the definition.
type SchemaRef struct {
	URI    string
	Inline map[string]any
}

// Provenance records where records in this datasource come from.
type Provenance struct {
	Source string
	URI    string
	Labels map[string]string
}

// CredentialRef references credentials by name without embedding secret values.
type CredentialRef struct {
	Name string
	Kind string
}

// CheckpointSpec describes cursor/checkpoint state used by sync-like actions.
type CheckpointSpec struct {
	CursorField string
	StateKey    string
}

// FreshnessSpec documents expected staleness or consistency characteristics.
type FreshnessSpec struct {
	TTL         string
	Consistency string
}

// Actions references standard action implementations associated with a
// datasource. Empty refs simply mean the datasource does not provide that
// operation.
type Actions struct {
	Fetch     action.Ref
	List      action.Ref
	Search    action.Ref
	Sync      action.Ref
	Map       action.Ref
	Transform action.Ref
}

// All returns non-empty action refs in stable standard-operation order.
func (a Actions) All() []action.Ref {
	refs := []action.Ref{a.Fetch, a.List, a.Search, a.Sync, a.Map, a.Transform}
	out := refs[:0]
	for _, ref := range refs {
		if !ref.IsZero() {
			out = append(out, ref)
		}
	}
	return out
}
