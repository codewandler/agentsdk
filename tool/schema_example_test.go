package tool

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSchemaOutput_integerFields verifies that integer fields have proper type annotations
// without injecting synthetic examples into the schema.
func TestSchemaOutput_integerFields(t *testing.T) {
	tl := New("test_tool", "A test tool.", func(ctx Ctx, p struct {
		Path  string `json:"path" jsonschema:"description=File path,required"`
		Limit int    `json:"limit,omitempty" jsonschema:"description=Max lines to read"`
	}) (Result, error) {
		return Text("ok"), nil
	})

	schema := tl.Schema()
	raw, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	t.Logf("Schema JSON:\n%s", string(raw))

	// Parse the schema to check properties
	var parsed map[string]any
	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	// Verify non-standard fields are removed
	_, hasSchema := parsed["$schema"]
	_, hasID := parsed["$id"]
	_, hasDefs := parsed["$defs"]
	require.False(t, hasSchema, "should not have $schema field")
	require.False(t, hasID, "should not have $id field")
	require.False(t, hasDefs, "should not have $defs field")

	props, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "should have properties")

	limitProp, ok := props["limit"].(map[string]any)
	require.True(t, ok, "should have limit property")

	// Check type is integer
	require.Equal(t, "integer", limitProp["type"], "limit should be integer type")

	_, hasExamples := limitProp["examples"]
	require.False(t, hasExamples, "schema projection should not inject synthetic examples")
}

// TestSchemaOutput_nestedIntegerFields verifies nested integer fields in arrays.
func TestSchemaOutput_nestedIntegerFields(t *testing.T) {
	tl := New("test_tool", "A test tool.", func(ctx Ctx, p struct {
		Ranges []struct {
			Start int `json:"start" jsonschema:"description=Start line,required"`
			End   int `json:"end,omitempty" jsonschema:"description=End line"`
		} `json:"ranges,omitempty" jsonschema:"description=Line ranges"`
	}) (Result, error) {
		return Text("ok"), nil
	})

	schema := tl.Schema()
	raw, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	t.Logf("Nested Schema JSON:\n%s", string(raw))

	// Check that ranges array items have integer fields
	var parsed map[string]any
	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	props := parsed["properties"].(map[string]any)
	ranges := props["ranges"].(map[string]any)
	items := ranges["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	require.Equal(t, "integer", itemProps["start"].(map[string]any)["type"])
	require.Equal(t, "integer", itemProps["end"].(map[string]any)["type"])

	startProp := itemProps["start"].(map[string]any)
	_, hasStartExamples := startProp["examples"]
	require.False(t, hasStartExamples, "schema projection should not inject synthetic examples")
}
