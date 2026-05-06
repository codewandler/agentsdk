package appconfig

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// AppDocument is the top-level schema type representing a single document in an
// agentsdk app config file. Multi-doc YAML files contain one or more of these.
// The "kind" field discriminates the document type; when omitted it defaults to
// "config".
type AppDocument struct {
	Kind Kind `json:"kind,omitempty" jsonschema:"enum=config,enum=agent,enum=command,enum=workflow,enum=action,enum=datasource,enum=trigger,default=config,description=Document kind discriminator"`

	// Config fields (kind=config)
	Name         string            `json:"name,omitempty" jsonschema:"description=Application name"`
	DefaultAgent string            `json:"default_agent,omitempty" jsonschema:"description=Default agent to use when none is specified"`
	Include      []string          `json:"include,omitempty" jsonschema:"description=Glob patterns for additional config files to include"`
	Resolution   *ResolutionConfig `json:"resolution,omitempty" jsonschema:"description=Resource resolution configuration"`
	Discovery    *DiscoveryConfig  `json:"discovery,omitempty" jsonschema:"description=Resource discovery policy"`
	Plugins      []PluginRef       `json:"plugins,omitempty" jsonschema:"description=Plugins to load"`

	// Agent fields (kind=agent)
	Description  string   `json:"description,omitempty" jsonschema:"description=Human-readable description"`
	Model        string   `json:"model,omitempty" jsonschema:"description=LLM model identifier"`
	MaxTokens    int      `json:"max_tokens,omitempty" jsonschema:"description=Maximum response tokens"`
	MaxSteps     int      `json:"max_steps,omitempty" jsonschema:"description=Maximum agentic loop steps"`
	Temperature  float64  `json:"temperature,omitempty" jsonschema:"description=Sampling temperature"`
	Thinking     string   `json:"thinking,omitempty" jsonschema:"description=Thinking mode (auto, always, none)"`
	Effort       string   `json:"effort,omitempty" jsonschema:"description=Reasoning effort level"`
	Tools        []string `json:"tools,omitempty" jsonschema:"description=Tool names to enable"`
	Skills       []string `json:"skills,omitempty" jsonschema:"description=Skill names to enable"`
	Commands     []string `json:"commands,omitempty" jsonschema:"description=Command names to expose"`
	Capabilities []string `json:"capabilities,omitempty" jsonschema:"description=Capability names to enable"`
	System       string   `json:"system,omitempty" jsonschema:"description=System prompt content"`

	// Command fields (kind=command)
	Target *CommandTarget `json:"target,omitempty" jsonschema:"description=Command execution target"`

	// Workflow fields (kind=workflow)
	Steps []any `json:"steps,omitempty" jsonschema:"description=Workflow step definitions"`

	// Action fields (kind=action)
	ActionKind string `json:"action_kind,omitempty" jsonschema:"description=Action type discriminator"`

	// Datasource fields (kind=datasource)
	DatasourceKind string `json:"datasource_kind,omitempty" jsonschema:"description=Datasource type discriminator"`

	// Trigger fields (kind=trigger)
	Source map[string]any `json:"source,omitempty" jsonschema:"description=Trigger source configuration"`
}

// GenerateJSONSchema returns the JSON Schema for the appconfig document format.
func GenerateJSONSchema() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		AllowAdditionalProperties: true,
	}
	return r.Reflect(&AppDocument{})
}

// GenerateJSONSchemaBytes returns the JSON Schema as indented JSON bytes.
func GenerateJSONSchemaBytes() ([]byte, error) {
	schema := GenerateJSONSchema()
	return json.MarshalIndent(schema, "", "  ")
}
