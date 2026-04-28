package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// minimalCtx is a test implementation of tool.Ctx (embeds context.Context + adds agentsdk-specific methods).
type minimalCtx struct {
	context.Context
}

func (c minimalCtx) WorkDir() string       { return "/tmp" }
func (c minimalCtx) AgentID() string       { return "test-agent" }
func (c minimalCtx) SessionID() string     { return "test-session" }
func (c minimalCtx) Extra() map[string]any { return nil }

// Test that TypedTool.Execute validates input before unmarshaling.
// Type errors are caught by validation with clear error messages.
func TestTypedTool_Execute_InvalidJSON_ReturnsHelpfulError(t *testing.T) {
	tl := New("my_tool", "A test tool.", func(ctx Ctx, p struct {
		Path  string   `json:"path"`
		Edits []string `json:"edits"` // expecting []string
	}) (Result, error) {
		return Text("ok"), nil
	})

	// Pass a string where an array is expected for "edits" field.
	// Validation catches this with a clear error message.
	raw := json.RawMessage(`{"path": "f.txt", "edits": "not an array"}`)

	_, err := tl.Execute(minimalCtx{Context: context.Background()}, raw)
	// The error should be returned as a Go error (infrastructure-level),
	// wrapping the validation failure with field context.
	require.Error(t, err)
	errMsg := err.Error()
	require.Contains(t, errMsg, "my_tool")
	// The error message should mention the field name and expected type.
	require.Contains(t, errMsg, "edits")
	require.Contains(t, errMsg, "array") // validation says "want array"
}

// Test that TypedTool.Execute returns helpful error for wrong type in nested field.
func TestTypedTool_Execute_InvalidJSON_WrongTypeNested(t *testing.T) {
	tl := New("patch_tool", "A patch tool.", func(ctx Ctx, p struct {
		Patch string `json:"patch"`
	}) (Result, error) {
		return Text("ok"), nil
	})

	// Pass a number where a string is expected.
	raw := json.RawMessage(`{"patch": 123}`)

	_, err := tl.Execute(minimalCtx{Context: context.Background()}, raw)
	require.Error(t, err)
	errMsg := err.Error()
	require.Contains(t, errMsg, "patch_tool")
	require.Contains(t, errMsg, "patch")
	require.Contains(t, errMsg, "string")
}

// Test that TypedTool.Execute handles empty input gracefully.
func TestTypedTool_Execute_EmptyInput_Succeeds(t *testing.T) {
	tl := New("minimal_tool", "A minimal tool.", func(ctx Ctx, p struct {
		Path string `json:"path"`
	}) (Result, error) {
		require.Equal(t, "", p.Path) // zero value
		return Text("ok"), nil
	})

	_, err := tl.Execute(minimalCtx{Context: context.Background()}, json.RawMessage(`{}`))
	require.NoError(t, err)
}

// Test that TypedTool.Execute handles null input gracefully.
func TestTypedTool_Execute_NullInput_Succeeds(t *testing.T) {
	tl := New("nullable_tool", "A nullable tool.", func(ctx Ctx, p struct {
		Path string `json:"path"`
	}) (Result, error) {
		return Text("ok"), nil
	})

	_, err := tl.Execute(minimalCtx{Context: context.Background()}, json.RawMessage(`null`))
	require.NoError(t, err)
}

// Test that parseError wraps json.UnmarshalTypeError with tool name and field context.
func TestParseError_UnmarshalTypeError(t *testing.T) {
	sliceType := reflect.TypeOf([]string(nil))
	err := parseError("file_edit", &json.UnmarshalTypeError{
		Value:  "string",
		Type:   sliceType,
		Field:  "edits",
		Offset: 42,
	})
	msg := err.Error()
	require.Contains(t, msg, "file_edit")
	require.Contains(t, msg, "edits")
	require.Contains(t, msg, "array<string>")
	require.Contains(t, msg, "string")
}

// Test that parseError falls back for unknown error types.
func TestParseError_UnknownErrorType(t *testing.T) {
	err := parseError("my_tool", fmt.Errorf("something went wrong"))
	msg := err.Error()
	require.Contains(t, msg, "my_tool")
	require.Contains(t, msg, "parse")
	require.Contains(t, msg, "something went wrong")
}

// Test that JSON Schema validation happens before unmarshaling
func TestTypedTool_JSONSchemaValidation_MissingRequired(t *testing.T) {
	t.Run("missing required field returns error", func(t *testing.T) {
		// Create a tool with a required "name" field
		type Params struct {
			Name string `json:"name" jsonschema:"description=Name,required"`
			Age  int    `json:"age" jsonschema:"description=Age"`
		}

		tk := New("test", "Test tool", func(ctx Ctx, p Params) (Result, error) {
			return Textf("Hello %s", p.Name), nil
		})

		// Missing required field - should return Go error
		_, err := tk.Execute(minimalCtx{Context: context.Background()}, json.RawMessage(`{"age": 30}`))

		require.Error(t, err, "Should return error for missing required field")
		require.Contains(t, err.Error(), "validate")
		require.Contains(t, err.Error(), "name")
	})
}

func TestTypedTool_JSONSchemaValidation_WrongType(t *testing.T) {
	t.Run("wrong type returns error", func(t *testing.T) {
		type Params struct {
			Name string `json:"name" jsonschema:"description=Name,required"`
			Age  int    `json:"age" jsonschema:"description=Age"`
		}

		tk := New("test", "Test tool", func(ctx Ctx, p Params) (Result, error) {
			return Textf("Hello %s", p.Name), nil
		})

		// Wrong type for name (should be string, not number)
		_, err := tk.Execute(minimalCtx{Context: context.Background()}, json.RawMessage(`{"name": 123}`))

		require.Error(t, err)
		require.Contains(t, err.Error(), "validate")
	})
}

func TestTypedTool_JSONSchemaValidation_ValidInput(t *testing.T) {
	t.Run("valid input succeeds", func(t *testing.T) {
		type Params struct {
			Name string `json:"name" jsonschema:"description=Name,required"`
			Age  int    `json:"age" jsonschema:"description=Age"`
		}

		tk := New("test", "Test tool", func(ctx Ctx, p Params) (Result, error) {
			return Textf("Hello %s", p.Name), nil
		})

		// Valid input
		res, err := tk.Execute(minimalCtx{Context: context.Background()}, json.RawMessage(`{"name": "Alice", "age": 30}`))

		require.NoError(t, err)
		require.False(t, res.IsError(), "Valid input should not return error")
	})
}

// ── hasRequiredToken + escaped comma tests ────────────────────────────────────

func TestHasRequiredToken(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want bool
	}{
		{"simple required", "required", true},
		{"required with description", "description=hello,required", true},
		{"required first", "required,description=hello", true},
		{"no required", "description=hello", false},
		{"escaped comma before required", "description=a\\, b,required", true},
		{"multiple escaped commas before required", "description=a\\, b\\, c,required", true},
		{"required-like inside value", "description=required field,minLength=1", false},
		{"empty tag", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasRequiredToken(tt.tag)
			require.Equal(t, tt.want, got, "hasRequiredToken(%q)", tt.tag)
		})
	}
}

// TestSchemaFor_EscapedCommaDescription verifies that a field whose type
// implements JSONSchema() still gets marked required and retains its
// description when the jsonschema tag contains escaped commas (\\,).
func TestSchemaFor_EscapedCommaDescription(t *testing.T) {
	// This struct mirrors the pattern in FileEditParams: a StringSliceParam
	// field (which implements JSONSchema()) with a description containing commas.
	type Params struct {
		Path StringSliceParam `json:"path" jsonschema:"description=Accepts a string\\, an array\\, or a glob.,required"`
		Name string           `json:"name" jsonschema:"description=Simple name,required"`
	}

	s := SchemaFor[Params]()

	// Both fields must be required
	require.Contains(t, s.Required, "path", "path must be required")
	require.Contains(t, s.Required, "name", "name must be required")

	// The description with commas must be preserved (commas unescaped in output)
	pathProp, ok := s.Properties.Get("path")
	require.True(t, ok, "path property must exist")
	require.Contains(t, pathProp.Description, "string, an array, or a glob",
		"escaped commas must appear as literal commas in the description")
}
