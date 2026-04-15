// Package tool — typed tool constructor (this file: tool.New, tool.NewResult, and helpers).
package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
	jsv "github.com/santhosh-tekuri/jsonschema/v6"

	llmtool "github.com/codewandler/llm/tool"
)

// TypedTool is a generic typed implementation of tool.Tool.
// P is the JSON-decodable parameter struct. Schema is auto-generated via
// llm/tool.DefinitionFor; the handler receives a fully decoded P on each call.
type TypedTool[P any] struct {
	name        string
	description string
	schema      *jsonschema.Schema // for LLM API
	validated   *jsv.Schema        // compiled for validation
	handler     func(ctx Ctx, p P) (Result, error)
	guidance    string // optional guidance shown in HEAD context
}

// New creates a typed Tool. The schema is derived from the zero value of P.
// Uses llm/tool.DefinitionFor for schema generation and compiles it for validation.
func New[P any](name, description string, handler func(ctx Ctx, p P) (Result, error), opts ...TypedToolOption[P]) *TypedTool[P] {
	// Use llm's DefinitionFor for clean schema generation
	def := llmtool.DefinitionFor[P](name, description)

	// Convert map[string]any to *jsonschema.Schema for LLM API
	raw, _ := json.Marshal(def.Parameters)
	var rawSchema jsonschema.Schema
	_ = json.Unmarshal(raw, &rawSchema)

	// Enhance the schema for LLM compatibility (add examples to numeric fields)
	schema := addExamplesToSchema(&rawSchema)

	// Compile map directly for validation (avoids Schema conversion issues)
	validated := compileMapForValidation(def.Parameters)

	t := &TypedTool[P]{
		name:        name,
		description: description,
		schema:      schema,
		validated:   validated,
		handler:     handler,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// addExamplesToSchema adds examples to integer/number fields to help LLMs send correct types.
func addExamplesToSchema(schema *jsonschema.Schema) *jsonschema.Schema {
	if schema == nil {
		return nil
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		return schema
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return schema
	}

	// Add examples to numeric fields recursively
	addExamplesToMap(m)

	cleanRaw, err := json.Marshal(m)
	if err != nil {
		return schema
	}

	var result jsonschema.Schema
	if err := json.Unmarshal(cleanRaw, &result); err != nil {
		return schema
	}

	return &result
}

// addExamplesToMap recursively adds examples to numeric fields in a map-based schema.
func addExamplesToMap(m map[string]any) {
	if m == nil {
		return
	}

	if typ, ok := m["type"].(string); ok {
		switch typ {
		case "integer":
			if _, has := m["example"]; !has {
				if examples, ok := m["examples"].([]any); !ok || len(examples) == 0 {
					m["examples"] = []any{10}
				}
			}
		case "number":
			if _, has := m["example"]; !has {
				if examples, ok := m["examples"].([]any); !ok || len(examples) == 0 {
					m["examples"] = []any{1.5}
				}
			}
		}
	}

	if props, ok := m["properties"].(map[string]any); ok {
		for _, prop := range props {
			if propMap, ok := prop.(map[string]any); ok {
				addExamplesToMap(propMap)
			}
		}
	}

	if items, ok := m["items"].(map[string]any); ok {
		addExamplesToMap(items)
	} else if items, ok := m["items"].([]any); ok {
		for _, item := range items {
			if itemMap, ok := item.(map[string]any); ok {
				addExamplesToMap(itemMap)
			}
		}
	}

	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if itemMap, ok := item.(map[string]any); ok {
					addExamplesToMap(itemMap)
				}
			}
		}
	}
}

// compileMapForValidation compiles a map[string]any (from llm DefinitionFor)
// into a santhosh-tekuri schema for runtime validation. Returns nil on failure.
func compileMapForValidation(params map[string]any) *jsv.Schema {
	if params == nil {
		return nil
	}

	c := jsv.NewCompiler()
	if err := c.AddResource("schema.json", params); err != nil {
		return nil
	}

	compiled, err := c.Compile("schema.json")
	if err != nil {
		return nil
	}

	return compiled
}

// TypedToolOption configures a TypedTool.
type TypedToolOption[P any] func(*TypedTool[P])

// WithGuidance sets the guidance string shown in HEAD context.
func WithGuidance[P any](g string) TypedToolOption[P] {
	return func(t *TypedTool[P]) { t.guidance = g }
}

func (t *TypedTool[P]) Name() string               { return t.name }
func (t *TypedTool[P]) Description() string        { return t.description }
func (t *TypedTool[P]) Schema() *jsonschema.Schema { return t.schema }
func (t *TypedTool[P]) Guidance() string           { return t.guidance }

func (t *TypedTool[P]) Execute(ctx Ctx, input json.RawMessage) (Result, error) {
	if len(input) == 0 || string(input) == "null" {
		return t.handler(ctx, *new(P))
	}

	// Validate input against compiled schema before unmarshaling
	if t.validated != nil {
		var args map[string]any
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, parseError(t.name, err)
		}
		if err := t.validated.Validate(args); err != nil {
			return nil, validationError(t.name, err)
		}
	}

	var p P
	if err := json.Unmarshal(input, &p); err != nil {
		return nil, parseError(t.name, err)
	}
	return t.handler(ctx, p)
}

// validationError wraps a jsonschema validation error with tool name context.
func validationError(toolName string, err error) error {
	return fmt.Errorf("validate %s input: %w", toolName, err)
}

// parseError wraps a JSON parse/unmarshal error with tool name and field context.
// Instead of a cryptic "cannot unmarshal string into Go struct field X of type Y",
// returns a structured message: "parse <tool>: <field> (expects <type>, got <actual>)".
func parseError(toolName string, err error) error {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		typeStr := typeName(ute.Type)
		field := ute.Field
		if field != "" {
			return fmt.Errorf("parse %s input: cannot unmarshal %s into field %q (expects %s, got %s)",
				toolName, ute.Value, field, typeStr, ute.Value)
		}
		return fmt.Errorf("parse %s input: cannot unmarshal %s (expects %s, got %s)",
			toolName, ute.Value, typeStr, ute.Value)
	}

	var sce *json.SyntaxError
	if errors.As(err, &sce) {
		return fmt.Errorf("parse %s input: invalid JSON at byte %d",
			toolName, sce.Offset)
	}

	// Generic fallback — still wrap with tool name for context.
	return fmt.Errorf("parse %s input: %w", toolName, err)
}

// typeName returns a human-readable name for a reflect.Type.
func typeName(t reflect.Type) string {
	if t == nil {
		return "unknown"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice:
		return fmt.Sprintf("array<%s>", typeName(t.Elem()))
	case reflect.Map:
		return fmt.Sprintf("map<%s,%s>", typeName(t.Key()), typeName(t.Elem()))
	case reflect.Ptr:
		return typeName(t.Elem())
	case reflect.Struct:
		return "object"
	case reflect.Interface:
		if t.NumMethod() == 0 {
			return "any"
		}
		return t.String()
	}
	// Fallback: full package path for custom types
	if t.PkgPath() != "" {
		return t.String()
	}
	return t.Name()
}
