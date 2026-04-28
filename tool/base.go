// Package tool — typed tool constructor (this file: tool.New, tool.NewResult, and helpers).
package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
	jsv "github.com/santhosh-tekuri/jsonschema/v6"
)

// TypedTool is a generic typed implementation of tool.Tool.
// P is the JSON-decodable parameter struct. Schema is auto-generated via
// local JSON Schema reflection; the handler receives a fully decoded P on each call.
type TypedTool[P any] struct {
	name        string
	description string
	schema      *jsonschema.Schema // for LLM API
	validated   *jsv.Schema        // compiled for validation
	handler     func(ctx Ctx, p P) (Result, error)
	guidance    string // optional guidance shown in HEAD context
}

// New creates a typed Tool. The schema is derived from the zero value of P.
// It uses local JSON Schema reflection and compiles the schema for validation.
func New[P any](name, description string, handler func(ctx Ctx, p P) (Result, error), opts ...TypedToolOption[P]) *TypedTool[P] {
	params, rawSchema := schemaFor[P]()

	// Enhance the schema for LLM compatibility (add examples to numeric fields)
	schema := addExamplesToSchema(rawSchema)

	// Compile map directly for validation (avoids Schema conversion issues).
	validated := compileMapForValidation(params)

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

func schemaFor[P any]() (map[string]any, *jsonschema.Schema) {
	r := jsonschema.Reflector{
		DoNotReference:             true,
		Anonymous:                  true,
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: true,
	}
	schema := r.Reflect(new(P))
	schema.Version = ""

	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, schema
	}

	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, schema
	}
	delete(params, "$schema")
	delete(params, "$id")

	params = injectRequiredFromTags[P](params)

	cleanRaw, err := json.Marshal(params)
	if err != nil {
		return params, schema
	}
	var cleanSchema jsonschema.Schema
	if err := json.Unmarshal(cleanRaw, &cleanSchema); err != nil {
		return params, schema
	}
	return params, &cleanSchema
}

// SchemaFor returns the JSON schema for type P, using the same reflector
// configuration as tool.New. Useful for building composite schemas (e.g.
// oneOf variants) without duplicating reflector setup.
func SchemaFor[P any]() *jsonschema.Schema {
	_, s := schemaFor[P]()
	return s
}

// injectRequiredFromTags patches the "required" array in the schema map for
// fields that carry jsonschema:"required" but whose types implement JSONSchema()
// — causing the reflector to skip their struct tags entirely.
func injectRequiredFromTags[P any](m map[string]any) map[string]any {
	var zero P
	t := reflect.TypeOf(zero)
	if t == nil {
		return m
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return m
	}
	var required []any
	for i := range t.NumField() {
		f := t.Field(i)
		for _, token := range strings.Split(f.Tag.Get("jsonschema"), ",") {
			if strings.TrimSpace(token) == "required" {
				if name := strings.Split(f.Tag.Get("json"), ",")[0]; name != "" && name != "-" {
					required = append(required, name)
				}
				break
			}
		}
	}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
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

// compileMapForValidation compiles a map[string]any
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
