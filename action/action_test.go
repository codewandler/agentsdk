package action

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type typedInput struct {
	Name string `json:"name" jsonschema:"required"`
	Age  int    `json:"age"`
}

type typedOutput struct {
	Greeting string `json:"greeting"`
}

func TestNewTypedExecutesAndInfersTypes(t *testing.T) {
	a := NewTyped[typedInput, typedOutput](Spec{
		Name:        "greet",
		Description: "Greet a person.",
	}, func(ctx Ctx, in typedInput) (typedOutput, error) {
		require.NotNil(t, ctx)
		return typedOutput{Greeting: "hello " + in.Name}, nil
	})

	spec := a.Spec()
	require.Equal(t, "greet", spec.Name)
	require.Equal(t, reflect.TypeOf(typedInput{}), spec.Input.GoType)
	require.Equal(t, reflect.TypeOf(typedOutput{}), spec.Output.GoType)
	require.NotNil(t, spec.Input.Schema)
	require.NotNil(t, spec.Output.Schema)

	result := a.Execute(context.Background(), typedInput{Name: "Ada", Age: 42})
	require.NoError(t, result.Error)
	require.Equal(t, typedOutput{Greeting: "hello Ada"}, result.Data)
}

func TestNewTypedMapsHandlerErrorToResult(t *testing.T) {
	want := errors.New("boom")
	a := NewTyped[typedInput, typedOutput](Spec{Name: "fail"}, func(Ctx, typedInput) (typedOutput, error) {
		return typedOutput{}, want
	})

	result := a.Execute(context.Background(), typedInput{Name: "Ada"})
	require.ErrorIs(t, result.Error, want)
	require.Nil(t, result.Data)
}

func TestNewTypedInvalidInputReturnsResultError(t *testing.T) {
	a := NewTyped[typedInput, typedOutput](Spec{Name: "greet"}, func(Ctx, typedInput) (typedOutput, error) {
		return typedOutput{}, nil
	})

	result := a.Execute(context.Background(), "not input")
	require.Error(t, result.Error)
	var invalid ErrInvalidInput
	require.ErrorAs(t, result.Error, &invalid)
	require.Equal(t, reflect.TypeOf(typedInput{}), invalid.Expected)
	require.Equal(t, reflect.TypeOf(""), invalid.Actual)
}

func TestTypeDecodeJSONValidatesWhenSchemaPresent(t *testing.T) {
	typ := TypeOf[typedInput]()
	require.NotNil(t, typ.Schema)

	decoded, err := typ.DecodeJSON([]byte(`{"name":"Ada","age":42}`))
	require.NoError(t, err)
	require.Equal(t, typedInput{Name: "Ada", Age: 42}, decoded)

	_, err = typ.DecodeJSON([]byte(`{"age":42}`))
	require.Error(t, err)
}

func TestTypeSchemaNilForNonSerializableInput(t *testing.T) {
	typ := TypeOf[chan int]()
	require.Equal(t, reflect.TypeOf((chan int)(nil)), typ.GoType)
	require.Nil(t, typ.Schema)
}

func TestRegistryRegistersAndResolvesActions(t *testing.T) {
	a := New(Spec{Name: "one"}, func(Ctx, any) Result { return Result{Data: "ok"} })
	reg := NewRegistry()
	require.NoError(t, reg.Register(a))

	got, ok := reg.Get("one")
	require.True(t, ok)
	require.Same(t, a, got)
	require.Equal(t, []Action{a}, reg.All())
	require.ErrorAs(t, reg.Register(a), &ErrDuplicate{})
}

func TestMiddlewareWrapsExecution(t *testing.T) {
	a := New(Spec{Name: "base"}, func(Ctx, any) Result { return Result{Data: "base"} })
	wrapped := Apply(a, MiddlewareFunc(func(next Handler) Handler {
		return func(ctx Ctx, input any) Result {
			result := next(ctx, input)
			result.Events = append(result.Events, "wrapped")
			return result
		}
	}))

	result := wrapped.Execute(context.Background(), nil)
	require.Equal(t, "base", result.Data)
	require.Equal(t, []Event{"wrapped"}, result.Events)
}
