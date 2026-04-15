package shell

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBashParams_Unmarshal_singleCommand(t *testing.T) {
	data := `{"cmd": "echo hello"}`
	var p BashParams
	err := json.Unmarshal([]byte(data), &p)
	require.NoError(t, err)
	require.Len(t, p.Cmd, 1)
	require.Equal(t, "echo hello", p.Cmd[0])
}

func TestBashParams_Unmarshal_multipleCommands(t *testing.T) {
	data := `{"cmd": ["echo hello", "echo world"]}`
	var p BashParams
	err := json.Unmarshal([]byte(data), &p)
	require.NoError(t, err)
	require.Len(t, p.Cmd, 2)
	require.Equal(t, "echo hello", p.Cmd[0])
	require.Equal(t, "echo world", p.Cmd[1])
}

func TestBashParams_Unmarshal_FailFast(t *testing.T) {
	data := `{"cmd": ["echo hello"], "failfast": true}`
	var p BashParams
	err := json.Unmarshal([]byte(data), &p)
	require.NoError(t, err)
	require.True(t, p.FailFast)
}

func TestBash_Schema_hasOneOf(t *testing.T) {
	tools := Tools()
	require.Len(t, tools, 1)

	schema := tools[0].Schema()
	require.NotNil(t, schema)

	// Convert to JSON to inspect the schema structure
	b, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(b, &result)
	require.NoError(t, err)

	// Navigate to the cmd property
	props, ok := result["properties"].(map[string]any)
	require.True(t, ok, "schema should have properties")

	cmdProp, ok := props["cmd"].(map[string]any)
	require.True(t, ok, "schema should have cmd property")

	// Check that cmd has oneOf
	oneOf, ok := cmdProp["oneOf"].([]any)
	require.True(t, ok, "cmd property should have oneOf")
	require.Len(t, oneOf, 2, "cmd oneOf should have exactly 2 variants")

	// First variant should be string
	variant1, ok := oneOf[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "string", variant1["type"], "first variant should be string")

	// Second variant should be array
	variant2, ok := oneOf[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "array", variant2["type"], "second variant should be array")
}
