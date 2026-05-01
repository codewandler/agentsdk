package action

// Intent describes what an action call is about to do, expressed as operations
// on resources. Risk assessment, approval gates, audit systems, and workflow
// policy consume this surface-neutral representation.
type Intent struct {
	// Action is the executable action name.
	Action string `json:"action,omitempty"`

	// Tool is kept for tool-package compatibility during migration. New action
	// code should populate Action instead.
	Tool string `json:"tool,omitempty"`

	// Class is the static intent category of the action itself, independent of
	// parameters. Examples: command_execution, filesystem_read,
	// filesystem_write, network_access, repository_access, vision, agent_control.
	Class string `json:"class,omitempty"`

	// ToolClass is kept for tool-package compatibility during migration. New
	// action code should populate Class instead.
	ToolClass string `json:"tool_class,omitempty"`

	// Operations is the set of resource+operation pairs this specific call will
	// perform. It is derived from actual params at call time.
	Operations []IntentOperation `json:"operations,omitempty"`

	// Behaviors are high-level semantic labels such as filesystem_read or
	// network_fetch.
	Behaviors []string `json:"behaviors,omitempty"`

	// Confidence indicates how certain the intent extraction is: high, moderate,
	// or low.
	Confidence string `json:"confidence"`

	// Opaque is true when semantics could not be determined. Risk assessors
	// should treat opaque intents conservatively.
	Opaque bool `json:"opaque,omitempty"`

	// Summary is a human-readable description of what this call will do.
	Summary string `json:"summary,omitempty"`

	// Extra carries action-specific data that downstream consumers can type-assert.
	// It is intentionally not serialized.
	Extra any `json:"-"`
}

// IntentOperation is a single resource+operation pair.
type IntentOperation struct {
	// Resource identifies what is being acted upon.
	Resource IntentResource `json:"resource"`

	// Operation is the action: read, write, delete, execute, network_read,
	// network_write, persistence_modify, device_write, etc.
	Operation string `json:"operation"`

	// Certain indicates whether this operation is definitely happening or only
	// inferred/conditional.
	Certain bool `json:"certain"`
}

// IntentResource identifies a resource being acted upon.
type IntentResource struct {
	// Category: file, directory, url, host, service, device, process, repo,
	// config, secret, environment_variable.
	Category string `json:"category"`

	// Value is the concrete identifier: path, URL, hostname, etc.
	Value string `json:"value"`

	// Locality: workspace, sensitive, secret, system, network, unknown.
	Locality string `json:"locality"`
}

// IntentProvider is an optional interface an Action can implement to declare
// what it is about to do before execution. DeclareIntent must be side-effect-free
// and fast.
type IntentProvider interface {
	DeclareIntent(ctx Ctx, input any) (Intent, error)
}

// ExtractIntent returns an action's declared intent and lets action middleware
// layers amend it from inside out.
func ExtractIntent(a Action, ctx Ctx, input any) Intent {
	target := Innermost(a)
	var intent Intent
	if provider, ok := target.(IntentProvider); ok {
		var err error
		intent, err = provider.DeclareIntent(ctx, input)
		if err != nil {
			intent = opaqueIntent(target)
		}
	} else {
		intent = opaqueIntent(target)
	}
	if intent.Action == "" && target != nil {
		intent.Action = target.Spec().Name
	}
	if intent.Class == "" && intent.ToolClass != "" {
		intent.Class = intent.ToolClass
	}

	layers := hookLayers(a)
	for _, layer := range layers {
		intent = layer.hooks.OnIntent(ctx, layer.inner, intent, nil)
	}
	return intent
}

func opaqueIntent(a Action) Intent {
	name := ""
	if a != nil {
		name = a.Spec().Name
	}
	return Intent{Action: name, Class: "unknown", Opaque: true, Confidence: "low"}
}
