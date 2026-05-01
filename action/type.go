package action

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
	jsv "github.com/santhosh-tekuri/jsonschema/v6"
)

// Type describes an action input or output contract.
type Type struct {
	GoType reflect.Type
	Schema *jsonschema.Schema
}

// TypeOf returns a Type for T. Schema is nil when T is not suitable for JSON
// schema projection, such as channels, functions, or interfaces.
func TypeOf[T any]() Type {
	goType := reflect.TypeOf((*T)(nil)).Elem()
	return Type{GoType: goType, Schema: SchemaForType(goType)}
}

// IsZero reports whether t has no type or schema metadata.
func (t Type) IsZero() bool {
	return t.GoType == nil && t.Schema == nil
}

// New creates a new pointer value suitable for decoding into t.GoType.
func (t Type) New() (any, error) {
	if t.GoType == nil {
		return nil, fmt.Errorf("action: type is not set")
	}
	if t.GoType.Kind() == reflect.Ptr {
		return reflect.New(t.GoType.Elem()).Interface(), nil
	}
	return reflect.New(t.GoType).Interface(), nil
}

// EncodeJSON serializes value as JSON.
func (t Type) EncodeJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

// DecodeJSON validates data against Schema when present, then decodes it into
// the Go type represented by t.
func (t Type) DecodeJSON(data []byte) (any, error) {
	if err := t.ValidateJSON(data); err != nil {
		return nil, err
	}
	ptr, err := t.New()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, ptr); err != nil {
		return nil, err
	}
	if t.GoType.Kind() == reflect.Ptr {
		return ptr, nil
	}
	return reflect.ValueOf(ptr).Elem().Interface(), nil
}

// ValidateJSON validates data against Schema. If no schema is available,
// validation is a no-op.
func (t Type) ValidateJSON(data []byte) error {
	if t.Schema == nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	raw, err := json.Marshal(t.Schema)
	if err != nil {
		return err
	}
	var schemaMap map[string]any
	if err := json.Unmarshal(raw, &schemaMap); err != nil {
		return err
	}
	delete(schemaMap, "$schema")
	delete(schemaMap, "$id")
	compiler := jsv.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaMap); err != nil {
		return err
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	return compiled.Validate(decoded)
}

func jsonSchemaEligible(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == nil {
		return false
	}
	if seen[t] {
		return true
	}
	seen[t] = true
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	case reflect.Slice, reflect.Array:
		return jsonSchemaEligible(t.Elem(), seen)
	case reflect.Map:
		return t.Key().Kind() == reflect.String && jsonSchemaEligible(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" { // unexported
				continue
			}
			if field.Tag.Get("json") == "-" {
				continue
			}
			if !jsonSchemaEligible(field.Type, seen) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
