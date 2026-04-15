package tool

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypedTool_Execute_ValidationError_MissingRequired(t *testing.T) {
	type Params struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age"`
	}

	var called bool
	myTool := New[Params]("test_tool", "Test tool",
		func(ctx Ctx, p Params) (Result, error) {
			called = true
			return Text("ok"), nil
		},
	)

	// Valid input - should call handler
	_, err := myTool.Execute(nil, json.RawMessage(`{"name":"Alice","age":25}`))
	require.NoError(t, err)
	require.True(t, called)

	// Invalid: missing required field - should NOT call handler
	called = false
	_, err = myTool.Execute(nil, json.RawMessage(`{"age":25}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate test_tool input")
	require.Contains(t, err.Error(), "name")
	require.False(t, called)
}

func TestTypedTool_Execute_ValidationError_WrongType(t *testing.T) {
	type Params struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age"`
	}

	var called bool
	myTool := New[Params]("test_tool", "Test tool",
		func(ctx Ctx, p Params) (Result, error) {
			called = true
			return Text("ok"), nil
		},
	)

	// Invalid: wrong type for age (string instead of int)
	called = false
	_, err := myTool.Execute(nil, json.RawMessage(`{"name":"Bob","age":"notanint"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate test_tool input")
	require.False(t, called)
}

func TestTypedTool_Execute_ValidationError_Enum(t *testing.T) {
	type Params struct {
		Unit string `json:"unit" jsonschema:"required,enum=celsius,enum=fahrenheit"`
	}

	myTool := New[Params]("test", "Test",
		func(ctx Ctx, p Params) (Result, error) {
			return Text("ok"), nil
		},
	)

	// Valid enum value
	_, err := myTool.Execute(nil, json.RawMessage(`{"unit":"celsius"}`))
	require.NoError(t, err)

	// Invalid enum value
	_, err = myTool.Execute(nil, json.RawMessage(`{"unit":"kelvin"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate test input")
}

func TestTypedTool_Execute_ValidationError_NumericRange(t *testing.T) {
	type Params struct {
		Age int `json:"age" jsonschema:"required,minimum=0,maximum=120"`
	}

	myTool := New[Params]("test", "Test",
		func(ctx Ctx, p Params) (Result, error) {
			return Text("ok"), nil
		},
	)

	// Valid range
	_, err := myTool.Execute(nil, json.RawMessage(`{"age":25}`))
	require.NoError(t, err)

	// Below minimum
	_, err = myTool.Execute(nil, json.RawMessage(`{"age":-1}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate test input")

	// Above maximum
	_, err = myTool.Execute(nil, json.RawMessage(`{"age":150}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate test input")
}

func TestTypedTool_Execute_ValidationError_ArrayItems(t *testing.T) {
	type Params struct {
		Tags []string `json:"tags" jsonschema:"required"`
	}

	myTool := New[Params]("test", "Test",
		func(ctx Ctx, p Params) (Result, error) {
			return Text("ok"), nil
		},
	)

	// Valid array
	_, err := myTool.Execute(nil, json.RawMessage(`{"tags":["go","rust"]}`))
	require.NoError(t, err)

	// Invalid: array with wrong item type
	_, err = myTool.Execute(nil, json.RawMessage(`{"tags":[1,2,3]}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate test input")
}

func TestTypedTool_Execute_NoValidationOnNilSchema(t *testing.T) {
	// Tools without a schema should still work (backward compatibility)
	type Params struct {
		Name string `json:"name"`
	}

	var called bool
	myTool := New[Params]("test_tool", "Test tool",
		func(ctx Ctx, p Params) (Result, error) {
			called = true
			return Text("ok"), nil
		},
	)

	// Even with missing fields (no jsonschema:"required"), should work
	_, err := myTool.Execute(nil, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, called)
}
