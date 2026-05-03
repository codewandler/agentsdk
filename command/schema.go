package command

// JSONSchema is a small JSON Schema-compatible projection of command input
// descriptors. It intentionally models only the subset needed for command
// discovery surfaces.
type JSONSchema struct {
	Type        string                `json:"type,omitempty"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]JSONSchema `json:"properties,omitempty"`
	Items       *JSONSchema           `json:"items,omitempty"`
	Required    []string              `json:"required,omitempty"`
	Enum        []string              `json:"enum,omitempty"`
}

// CommandInputSchema projects a command descriptor's structured input fields
// into a small JSON Schema-compatible object schema. The command descriptor
// remains the source of truth; this schema is only a presentation/API projection.
func CommandInputSchema(desc Descriptor) JSONSchema {
	schema := JSONSchema{Type: "object"}
	if len(desc.Input.Fields) == 0 {
		return schema
	}

	schema.Properties = make(map[string]JSONSchema, len(desc.Input.Fields))
	for _, field := range desc.Input.Fields {
		fieldSchema := JSONSchema{
			Type:        jsonSchemaType(field.Type),
			Description: field.Description,
		}
		if field.Type == InputTypeArray {
			fieldSchema.Items = &JSONSchema{Type: "string"}
		}
		if len(field.EnumValues) > 0 {
			fieldSchema.Enum = append([]string(nil), field.EnumValues...)
		}
		schema.Properties[field.Name] = fieldSchema
		if field.Required {
			schema.Required = append(schema.Required, field.Name)
		}
	}
	return schema
}

func jsonSchemaType(inputType InputType) string {
	switch inputType {
	case InputTypeBool:
		return "boolean"
	case InputTypeInteger:
		return "integer"
	case InputTypeNumber:
		return "number"
	case InputTypeArray:
		return "array"
	default:
		return "string"
	}
}

// IsZero reports whether the schema carries no projection metadata.
func (s JSONSchema) IsZero() bool {
	return s.Type == "" && s.Description == "" && len(s.Properties) == 0 && s.Items == nil && len(s.Required) == 0 && len(s.Enum) == 0
}
