package tool

// Intent describes what a tool call is about to do, expressed as
// operations on resources. This is the abstract layer that risk
// assessment, approval gates, and audit systems consume.
//
// Full implementation in Phase 2. This file provides the type
// definitions so that Approver and middleware signatures compile now.
type Intent struct {
	// Tool is the tool name.
	Tool string `json:"tool"`

	// ToolClass is the static intent category of the tool itself,
	// independent of parameters. Known at registration time.
	ToolClass string `json:"tool_class"`

	// Operations is the set of resource+operation pairs this specific
	// call will perform.
	Operations []IntentOperation `json:"operations,omitempty"`

	// Behaviors are high-level semantic labels (e.g. filesystem_read,
	// network_fetch). Same vocabulary as cmdrisk.
	Behaviors []string `json:"behaviors,omitempty"`

	// Confidence indicates how certain the intent extraction is.
	//   "high"     — fully determined from typed params
	//   "moderate" — mostly determined, some inference
	//   "low"      — heuristic or opaque
	Confidence string `json:"confidence"`

	// Opaque is true when the tool's semantics could not be determined.
	Opaque bool `json:"opaque,omitempty"`

	// Extra carries tool-specific data that downstream consumers
	// (assessors, audit) can type-assert. Not serialized to JSON.
	Extra any `json:"-"`
}

// IntentOperation is a single resource+operation pair.
type IntentOperation struct {
	Resource  IntentResource `json:"resource"`
	Operation string         `json:"operation"`
	Certain   bool           `json:"certain"`
}

// IntentResource identifies a resource being acted upon.
type IntentResource struct {
	Category string `json:"category"`
	Value    string `json:"value"`
	Locality string `json:"locality"`
}
