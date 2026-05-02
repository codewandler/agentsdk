package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type typedCommandStatus string

type typedCommandInput struct {
	Name     string             `command:"arg=name"`
	Input    string             `command:"arg=input"`
	Values   []string           `command:"arg=values"`
	Workflow string             `command:"flag=workflow"`
	Status   typedCommandStatus `command:"flag=status"`
	Limit    int                `command:"flag=limit"`
	Verbose  bool               `command:"flag=verbose"`
	Ignored  string
}

type typedCommandPtrInput struct {
	Name *string `command:"arg=name"`
}

func TestTypedBindsInvocationToStruct(t *testing.T) {
	inv := Invocation{
		Path: []string{"workflow", "runs"},
		Args: map[string][]string{
			"name":   {"ask_flow"},
			"input":  {"hello", "world"},
			"values": {"a", "b"},
		},
		Flags: map[string]string{
			"workflow": "ask_flow",
			"status":   "failed",
			"limit":    "5",
			"verbose":  "true",
		},
	}

	got, err := Bind[typedCommandInput](inv)

	require.NoError(t, err)
	require.Equal(t, "ask_flow", got.Name)
	require.Equal(t, "hello world", got.Input)
	require.Equal(t, []string{"a", "b"}, got.Values)
	require.Equal(t, "ask_flow", got.Workflow)
	require.Equal(t, typedCommandStatus("failed"), got.Status)
	require.Equal(t, 5, got.Limit)
	require.True(t, got.Verbose)
	require.Empty(t, got.Ignored)
}

func TestTypedBindsPointerInputAndPointerFields(t *testing.T) {
	got, err := Bind[*typedCommandPtrInput](Invocation{Args: map[string][]string{"name": {"ask_flow"}}})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Name)
	require.Equal(t, "ask_flow", *got.Name)
}

func TestTypedExecutesHandlerWithBoundInput(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("runs", Typed(func(_ context.Context, input typedCommandInput) (Result, error) {
			return Text(fmt.Sprintf("%s:%s", input.Workflow, input.Status)), nil
		}),
			Flag("workflow"),
			Flag("status").Enum("running", "succeeded", "failed"),
		).
		Build()
	require.NoError(t, err)

	_, params, err := Parse("/workflow runs --workflow ask_flow --status failed")
	require.NoError(t, err)
	result, err := tree.Execute(context.Background(), params)

	require.NoError(t, err)
	require.Equal(t, "ask_flow:failed", renderCommandResult(t, result))
}

func TestTypedPropagatesHandlerError(t *testing.T) {
	want := errors.New("boom")
	handler := Typed(func(context.Context, typedCommandInput) (Result, error) {
		return Result{}, want
	})

	_, err := handler(context.Background(), Invocation{})

	require.ErrorIs(t, err, want)
}

func TestTypedRejectsMalformedTagAndUnsupportedType(t *testing.T) {
	t.Run("malformed tag", func(t *testing.T) {
		type bad struct {
			Name string `command:"arg"`
		}
		_, err := Bind[bad](Invocation{})
		var validation ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, ValidationInvalidSpec, validation.Code)
		require.Equal(t, "Name", validation.Field)
	})

	t.Run("unsupported field", func(t *testing.T) {
		type bad struct {
			Value map[string]string `command:"flag=value"`
		}
		_, err := Bind[bad](Invocation{Flags: map[string]string{"value": "x"}})
		var validation ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, ValidationInvalidFlagValue, validation.Code)
		require.Equal(t, "value", validation.Field)
	})
}

func TestTypedUsesArgValidationCodeForArgBindingFailure(t *testing.T) {
	type bad struct {
		Value map[string]string `command:"arg=value"`
	}

	_, err := Bind[bad](Invocation{Args: map[string][]string{"value": {"x"}}})

	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, ValidationInvalidArgValue, validation.Code)
	require.Equal(t, "value", validation.Field)
}

func TestTypedRejectsNonStructInput(t *testing.T) {
	_, err := Bind[string](Invocation{})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, ValidationInvalidSpec, validation.Code)
}

type typedCommandHintInput struct {
	Name    string             `command:"arg=name"`
	Status  typedCommandStatus `command:"flag=status"`
	Limit   int                `command:"flag=limit"`
	Verbose bool               `command:"flag=verbose"`
	Ratio   float64            `command:"flag=ratio"`
	Values  []string           `command:"arg=values"`
}

func TestTypedInputHintsInferDescriptorTypes(t *testing.T) {
	hints, err := InputHints[typedCommandHintInput]()

	require.NoError(t, err)
	require.Equal(t, []InputFieldDescriptor{
		{Name: "name", Source: InputSourceArg, Type: InputTypeString},
		{Name: "status", Source: InputSourceFlag, Type: InputTypeString},
		{Name: "limit", Source: InputSourceFlag, Type: InputTypeInteger},
		{Name: "verbose", Source: InputSourceFlag, Type: InputTypeBool},
		{Name: "ratio", Source: InputSourceFlag, Type: InputTypeNumber},
		{Name: "values", Source: InputSourceArg, Type: InputTypeArray},
	}, hints)
}

func TestTypedInputHintsRejectInvalidInput(t *testing.T) {
	t.Run("non struct", func(t *testing.T) {
		_, err := InputHints[string]()
		var validation ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, ValidationInvalidSpec, validation.Code)
	})

	t.Run("unsupported field", func(t *testing.T) {
		type bad struct {
			Value map[string]string `command:"flag=value"`
		}

		_, err := NewTree("bad", TypedInput[bad]()).Build()

		var validation ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, ValidationInvalidSpec, validation.Code)
		require.Equal(t, "Value", validation.Field)
	})
}
