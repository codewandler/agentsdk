// Package tool — typed tool constructor (this file: tool.New, tool.NewResult, and helpers).
package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/codewandler/agentsdk/action"
	"github.com/invopop/jsonschema"
	jsv "github.com/santhosh-tekuri/jsonschema/v6"
)

// TypedTool is a generic typed implementation of tool.Tool.
// P is the JSON-decodable parameter struct. Schema is auto-generated via
// local JSON Schema reflection; the handler receives a fully decoded P on each call.
type TypedTool[P any] struct {
	name          string
	description   string
	schema        *jsonschema.Schema // for LLM API
	validated     *jsv.Schema        // compiled for validation
	handler       func(ctx Ctx, p P) (Result, error)
	guidance      string // optional guidance shown in HEAD context
	declareIntent func(ctx Ctx, p P) (Intent, error)
}

// New creates a typed Tool. The schema is derived from the zero value of P.
// It uses local JSON Schema reflection and compiles the schema for validation.
func New[P any](name, description string, handler func(ctx Ctx, p P) (Result, error), opts ...TypedToolOption[P]) *TypedTool[P] {
	params, schema := schemaFor[P]()

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
	schema := action.SchemaFor[P]()
	params := action.SchemaMapFor[P]()
	return params, schema
}

// SchemaFor returns the JSON schema for type P. Deprecated: use action.SchemaFor.
func SchemaFor[P any]() *jsonschema.Schema { return action.SchemaFor[P]() }

// hasRequiredToken is kept for compatibility with older package tests. Deprecated:
// use action.HasRequiredToken.
func hasRequiredToken(tag string) bool { return action.HasRequiredToken(tag) }

// injectRequiredFromTags is kept for compatibility with older package tests.
// Deprecated: use action.InjectRequiredFromTags.
func injectRequiredFromTags[P any](m map[string]any) map[string]any {
	return action.InjectRequiredFromTags(reflect.TypeOf((*P)(nil)).Elem(), m)
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

// WithDeclareIntent sets the intent declaration function for a TypedTool.
// When set, the tool implements [IntentProvider] and can participate in
// risk assessment and approval flows.
//
// The function receives the already-decoded params (same as the handler)
// and returns the intent describing what the tool call will do.
func WithDeclareIntent[P any](fn func(ctx Ctx, p P) (Intent, error)) TypedToolOption[P] {
	return func(t *TypedTool[P]) { t.declareIntent = fn }
}

func (t *TypedTool[P]) Name() string               { return t.name }
func (t *TypedTool[P]) Description() string        { return t.description }
func (t *TypedTool[P]) Schema() *jsonschema.Schema { return t.schema }
func (t *TypedTool[P]) Guidance() string           { return t.guidance }

// DeclareIntent implements [IntentProvider]. If [WithDeclareIntent] was not
// set, it returns an opaque low-confidence intent.
//
// Note: TypedTool always satisfies IntentProvider. Tools without
// WithDeclareIntent return opaque intents, which is functionally
// identical to not implementing IntentProvider at all.
// Input is not schema-validated here (unlike Execute) — DeclareIntent
// is intentionally lenient and falls back to opaque on parse errors.
func (t *TypedTool[P]) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
	if t.declareIntent == nil {
		return Intent{
			Tool:       t.name,
			ToolClass:  "unknown",
			Opaque:     true,
			Confidence: "low",
		}, nil
	}

	var p P
	if len(input) > 0 && string(input) != "null" {
		if err := json.Unmarshal(input, &p); err != nil {
			return Intent{Tool: t.name, ToolClass: "unknown", Opaque: true, Confidence: "low"}, nil
		}
	}
	return t.declareIntent(ctx, p)
}

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
