package action

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
)

// SchemaFor returns the JSON schema for T when T can be projected to JSON.
// It uses the same schema projection rules as TypeOf[T].Schema.
func SchemaFor[T any]() *jsonschema.Schema {
	goType := reflect.TypeOf((*T)(nil)).Elem()
	return SchemaForType(goType)
}

// SchemaForType returns the JSON schema for t when t can be projected to JSON.
// It returns nil for Go-native types that do not have a useful JSON shape, such
// as channels, functions, and non-empty interfaces.
func SchemaForType(t reflect.Type) *jsonschema.Schema {
	if t == nil || !jsonSchemaEligible(t, map[reflect.Type]bool{}) {
		return nil
	}
	reflector := jsonschema.Reflector{
		DoNotReference:             true,
		Anonymous:                  true,
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: true,
	}
	ptr := reflect.New(t)
	if t.Kind() == reflect.Ptr {
		ptr = reflect.New(t.Elem())
	}
	schema := reflector.Reflect(ptr.Interface())
	if schema == nil {
		return nil
	}
	schema.Version = ""

	// Normalize through a map so we can remove reflector metadata and apply the
	// required-tag fix for fields whose type implements JSONSchema().
	m, err := schemaMap(schema)
	if err != nil {
		return schema
	}
	m = InjectRequiredFromTags(t, m)
	return schemaFromMap(m, schema)
}

// SchemaMapFor returns SchemaFor[T] as a map suitable for validation compilers.
func SchemaMapFor[T any]() map[string]any {
	schema := SchemaFor[T]()
	if schema == nil {
		return nil
	}
	m, err := schemaMap(schema)
	if err != nil {
		return nil
	}
	return m
}

// InjectRequiredFromTags patches the "required" array in schema maps for fields
// that carry jsonschema:"required" but whose types implement JSONSchema(), which
// causes the reflector to skip their struct tags. The function is exported so
// legacy projection layers can reuse the same behavior during migration.
func InjectRequiredFromTags(t reflect.Type, m map[string]any) map[string]any {
	if m == nil || t == nil {
		return m
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return m
	}
	var required []any
	if existing, ok := m["required"].([]any); ok {
		required = append(required, existing...)
	}
	seen := map[string]bool{}
	for _, v := range required {
		if s, ok := v.(string); ok {
			seen[s] = true
		}
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !HasRequiredToken(f.Tag.Get("jsonschema")) {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" || seen[name] {
			continue
		}
		required = append(required, name)
		seen[name] = true
	}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

// HasRequiredToken reports whether a jsonschema tag value contains "required"
// as a standalone comma-separated token, respecting \, escapes inside values.
func HasRequiredToken(tag string) bool {
	for len(tag) > 0 {
		i := 0
		for i < len(tag) {
			if tag[i] == '\\' {
				i += 2
				continue
			}
			if tag[i] == ',' {
				break
			}
			i++
		}
		token := strings.TrimSpace(tag[:i])
		if token == "required" {
			return true
		}
		if i >= len(tag) {
			break
		}
		tag = tag[i+1:]
	}
	return false
}

func schemaMap(schema *jsonschema.Schema) (map[string]any, error) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "$schema")
	delete(m, "$id")
	delete(m, "$defs")
	return m, nil
}

func schemaFromMap(m map[string]any, fallback *jsonschema.Schema) *jsonschema.Schema {
	raw, err := json.Marshal(m)
	if err != nil {
		return fallback
	}
	var out jsonschema.Schema
	if err := json.Unmarshal(raw, &out); err != nil {
		return fallback
	}
	return &out
}
