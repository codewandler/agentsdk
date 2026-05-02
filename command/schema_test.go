package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandInputSchemaEmptyDescriptor(t *testing.T) {
	schema := CommandInputSchema(Descriptor{})

	require.Equal(t, JSONSchema{Type: "object"}, schema)
}

func TestCommandInputSchemaProjectsDescriptorFields(t *testing.T) {
	schema := CommandInputSchema(Descriptor{
		Input: InputDescriptor{Fields: []InputFieldDescriptor{
			{Name: "name", Type: InputTypeString, Description: "Workflow name", Required: true},
			{Name: "limit", Type: InputTypeInteger},
			{Name: "verbose", Type: InputTypeBool},
			{Name: "ratio", Type: InputTypeNumber},
			{Name: "tags", Type: InputTypeArray},
			{Name: "status", Type: InputTypeString, EnumValues: []string{"running", "failed"}},
		}},
	})

	require.Equal(t, JSONSchema{
		Type: "object",
		Properties: map[string]JSONSchema{
			"name":    {Type: "string", Description: "Workflow name"},
			"limit":   {Type: "integer"},
			"verbose": {Type: "boolean"},
			"ratio":   {Type: "number"},
			"tags":    {Type: "array", Items: &JSONSchema{Type: "string"}},
			"status":  {Type: "string", Enum: []string{"running", "failed"}},
		},
		Required: []string{"name"},
	}, schema)
}

func TestCommandInputSchemaRendersAsJSON(t *testing.T) {
	schema := CommandInputSchema(Descriptor{
		Input: InputDescriptor{Fields: []InputFieldDescriptor{
			{Name: "name", Type: InputTypeString, Required: true},
			{Name: "input", Type: InputTypeArray},
		}},
	})

	text, err := Render(Display(schema), DisplayJSON)

	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"input": {"type": "array", "items": {"type": "string"}}
		},
		"required": ["name"]
	}`, text)
}

func TestCommandInputSchemaCopiesEnumValues(t *testing.T) {
	desc := Descriptor{Input: InputDescriptor{Fields: []InputFieldDescriptor{
		{Name: "status", Type: InputTypeString, EnumValues: []string{"running", "failed"}},
	}}}
	schema := CommandInputSchema(desc)
	schema.Properties["status"].Enum[0] = "mutated"

	require.Equal(t, "running", desc.Input.Fields[0].EnumValues[0])
}
