package action

import (
	"fmt"
	"reflect"
)

// ErrInvalidInput is returned in Result.Error when an action receives an input
// value that cannot be used as the action's expected input type.
type ErrInvalidInput struct {
	Expected reflect.Type
	Actual   reflect.Type
}

func (e ErrInvalidInput) Error() string {
	actual := "<nil>"
	if e.Actual != nil {
		actual = e.Actual.String()
	}
	if e.Expected == nil {
		return fmt.Sprintf("action: invalid input type %s", actual)
	}
	return fmt.Sprintf("action: invalid input type %s, expected %s", actual, e.Expected)
}

// CastInput converts input into I when possible. A nil input becomes I's zero
// value. Assignable and convertible values are accepted; incompatible values
// return ErrInvalidInput.
func CastInput[I any](input any) (I, error) {
	var zero I
	if input == nil {
		return zero, nil
	}
	if value, ok := input.(I); ok {
		return value, nil
	}
	expected := reflect.TypeOf((*I)(nil)).Elem()
	actual := reflect.TypeOf(input)
	if actual == nil {
		return zero, nil
	}
	value := reflect.ValueOf(input)
	if actual.AssignableTo(expected) {
		return value.Interface().(I), nil
	}
	if actual.ConvertibleTo(expected) {
		return value.Convert(expected).Interface().(I), nil
	}
	return zero, ErrInvalidInput{Expected: expected, Actual: actual}
}
