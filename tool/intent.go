package tool

import "encoding/json"

// Intent describes what a tool call is about to do, expressed as
// operations on resources. This is the abstract layer that risk
// assessment, approval gates, and audit systems consume.
type Intent struct {
	// Tool is the tool name.
	Tool string `json:"tool"`

	// ToolClass is the static intent category of the tool itself,
	// independent of parameters. Known at registration time.
	// Examples: "command_execution", "filesystem_read", "filesystem_write",
	// "filesystem_delete", "network_access", "repository_access", "vision",
	// "agent_control".
	ToolClass string `json:"tool_class"`

	// Operations is the set of resource+operation pairs this specific
	// call will perform. Derived from the actual params at call time.
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
	// Risk assessors should treat opaque intents conservatively.
	Opaque bool `json:"opaque,omitempty"`

	// Summary is a human-readable description of what this specific call
	// will do. For command_execution tools this is the shell command string.
	// Useful for audit logs, approval UIs, and assessors that need the
	// raw input (e.g. cmdrisk).
	Summary string `json:"summary,omitempty"`

	// Extra carries tool-specific data that downstream consumers
	// (assessors, audit) can type-assert. Not serialized to JSON.
	Extra any `json:"-"`
}

// IntentOperation is a single resource+operation pair.
type IntentOperation struct {
	// Resource identifies what is being acted upon.
	Resource IntentResource `json:"resource"`

	// Operation is the action: read, write, delete, execute,
	// network_read, network_write, persistence_modify, device_write.
	Operation string `json:"operation"`

	// Certain indicates whether this operation is definitely happening
	// (true) or inferred/conditional (false).
	Certain bool `json:"certain"`
}

// IntentResource identifies a resource being acted upon.
type IntentResource struct {
	// Category: file, directory, url, host, service, device, process,
	// repo, config, secret, environment_variable.
	Category string `json:"category"`

	// Value is the concrete identifier: path, URL, hostname, etc.
	Value string `json:"value"`

	// Locality: workspace, sensitive, secret, system, network, unknown.
	Locality string `json:"locality"`
}

// IntentProvider is an optional interface a Tool can implement to declare
// what it's about to do before execution. This enables risk assessment,
// approval gates, and audit without reverse-engineering tool semantics.
//
// Tools that don't implement IntentProvider are treated as opaque
// (Intent.Opaque = true, Confidence = "low").
type IntentProvider interface {
	// DeclareIntent inspects the raw input and returns the intent.
	// Called before Execute, with the same raw JSON.
	//
	// DeclareIntent must be side-effect-free and fast. It must not
	// perform the actual operation — only describe what would happen.
	DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error)
}

// ExtractIntent gets the intent from a tool, then walks the middleware
// chain outward letting each layer amend it via OnIntent.
//
// Flow:
//  1. Find the innermost tool via Unwrap chain.
//  2. If it implements IntentProvider, call DeclareIntent for the base intent.
//     Otherwise, create an opaque fallback intent.
//  3. Walk the wrapper chain from inside out, calling onIntent on each
//     hookedTool layer so middlewares can enrich/amend the intent.
//
// This means the inner tool declares "I will read file X", and an outer
// middleware can add "...and I will write an audit log to Y".
func ExtractIntent(t Tool, ctx Ctx, input json.RawMessage) Intent {
	// 1. Get base intent from innermost IntentProvider.
	target := Innermost(t)
	var intent Intent
	if provider, ok := target.(IntentProvider); ok {
		var err error
		intent, err = provider.DeclareIntent(ctx, input)
		if err != nil {
			intent = Intent{Tool: target.Name(), ToolClass: "unknown", Opaque: true, Confidence: "low"}
		}
	} else {
		intent = Intent{Tool: target.Name(), ToolClass: "unknown", Opaque: true, Confidence: "low"}
	}

	// 2. Collect middleware layers (outermost-first via Unwrap walk).
	var layers []*hookedTool
	cur := t
	for {
		if ht, ok := cur.(*hookedTool); ok {
			layers = append(layers, ht)
			cur = ht.inner
		} else {
			break
		}
	}

	// 3. Reverse to inside-out order: innermost middleware amends first,
	// outermost gets the final say.
	for i, j := 0, len(layers)-1; i < j; i, j = i+1, j-1 {
		layers[i], layers[j] = layers[j], layers[i]
	}

	// 4. Let each middleware layer amend the intent.
	// Each layer gets its own empty CallState — OnIntent is not part of
	// the Execute flow, so there's no shared state to carry.
	for _, ht := range layers {
		intent = ht.onIntent(ctx, intent, nil)
	}

	return intent
}
