// Package tool defines the interfaces and types for the flai tool system.
// Tools are capabilities the LLM can invoke. They are registered in a Registry,
// selectively activated, and called by the agent loop.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

// Ctx is the execution context passed to every tool call.
// It embeds context.Context (for cancellation/deadline) and provides
// agent/session metadata available at execution time.
type Ctx interface {
	context.Context
	WorkDir() string
	AgentID() string
	SessionID() string
	Extra() map[string]any
}

// Tool is a capability the LLM can invoke.
type Tool interface {
	Name() string
	Description() string
	Schema() *jsonschema.Schema
	Execute(ctx Ctx, input json.RawMessage) (Result, error)
	Guidance() string
}

// Registry holds registered tools and their activation state.
type Registry interface {
	Register(tools ...Tool) error
	Get(name string) (Tool, bool)
	All() []Tool
	Len() int
}

// ToolSpec is the LLM-facing description of a tool.
type ToolSpec struct {
	Name        string
	Description string
	Schema      *jsonschema.Schema
}

// SpecsFrom converts a slice of Tools to ToolSpecs.
func SpecsFrom(tools []Tool) []ToolSpec {
	specs := make([]ToolSpec, len(tools))
	for i, t := range tools {
		specs[i] = ToolSpec{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		}
	}
	return specs
}

// StringSliceParam is a Go type that accepts both a single string and an array
// of strings during JSON unmarshaling.
type StringSliceParam []string

// UnmarshalJSON implements json.Unmarshaler for StringSliceParam.
// It accepts both a single string and an array of strings.
func (p *StringSliceParam) UnmarshalJSON(data []byte) error {
	if len(data) == 4 && data[0] == 'n' {
		*p = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*p = []string{s}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("must be a string or array of strings, got %s", string(data))
	}
	*p = arr
	return nil
}

// JSONSchema implements the jsonschema.JSONSchemer interface.
// It returns a oneOf schema allowing either a single string or an array of strings.
func (StringSliceParam) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
		},
	}
}
