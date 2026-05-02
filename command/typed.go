package command

import (
	"context"
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Typed adapts a typed command handler to a tree handler. I must be a struct
// whose exported fields are tagged with command:"arg=name" or
// command:"flag=name".
func Typed[I any](fn func(context.Context, I) (Result, error)) TreeHandler {
	return func(ctx context.Context, inv Invocation) (Result, error) {
		input, err := Bind[I](inv)
		if err != nil {
			return Result{}, err
		}
		return fn(ctx, input)
	}
}

// Bind binds a validated invocation into I using command struct tags.
func Bind[I any](inv Invocation) (I, error) {
	var zero I
	value := reflect.New(reflect.TypeOf((*I)(nil)).Elem()).Elem()
	if err := bindValue(value, inv); err != nil {
		return zero, err
	}
	return value.Interface().(I), nil
}

func bindValue(value reflect.Value, inv Invocation) error {
	typ := value.Type()
	if typ.Kind() == reflect.Ptr {
		if typ.Elem().Kind() != reflect.Struct {
			return ValidationError{Path: inv.Path, Code: ValidationInvalidSpec, Message: fmt.Sprintf("command: typed input %s must point to a struct", typ)}
		}
		value.Set(reflect.New(typ.Elem()))
		return bindStruct(value.Elem(), inv)
	}
	if typ.Kind() != reflect.Struct {
		return ValidationError{Path: inv.Path, Code: ValidationInvalidSpec, Message: fmt.Sprintf("command: typed input %s must be a struct", typ)}
	}
	return bindStruct(value, inv)
}

func bindStruct(value reflect.Value, inv Invocation) error {
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		source, name, ok, err := parseCommandTag(field.Tag.Get("command"))
		if err != nil {
			return ValidationError{Path: inv.Path, Code: ValidationInvalidSpec, Field: field.Name, Message: err.Error()}
		}
		if !ok {
			continue
		}
		var values []string
		switch source {
		case "arg":
			values = inv.ArgValues(name)
		case "flag":
			if value := inv.Flag(name); value != "" {
				values = []string{value}
			}
		default:
			return ValidationError{Path: inv.Path, Code: ValidationInvalidSpec, Field: field.Name, Message: fmt.Sprintf("command: unsupported binding source %q", source)}
		}
		if len(values) == 0 {
			continue
		}
		if err := setFieldValue(value.Field(i), values); err != nil {
			return ValidationError{Path: inv.Path, Code: bindValidationCode(source), Field: name, Message: err.Error()}
		}
	}
	return nil
}

func bindValidationCode(source string) ValidationErrorCode {
	if source == "arg" {
		return ValidationInvalidArgValue
	}
	return ValidationInvalidFlagValue
}

func parseCommandTag(tag string) (source string, name string, ok bool, err error) {
	tag = strings.TrimSpace(tag)
	if tag == "" || tag == "-" {
		return "", "", false, nil
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return "", "", false, nil
	}
	binding := strings.TrimSpace(parts[0])
	key, value, found := strings.Cut(binding, "=")
	if !found || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return "", "", false, fmt.Errorf("command: malformed tag %q", tag)
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true, nil
}

func setFieldValue(field reflect.Value, values []string) error {
	if !field.CanSet() {
		return nil
	}
	if field.Kind() == reflect.Ptr {
		if len(values) == 0 {
			return nil
		}
		ptr := reflect.New(field.Type().Elem())
		if err := setFieldValue(ptr.Elem(), values); err != nil {
			return err
		}
		field.Set(ptr)
		return nil
	}
	if field.Kind() == reflect.Slice {
		slice := reflect.MakeSlice(field.Type(), 0, len(values))
		for _, value := range values {
			elem := reflect.New(field.Type().Elem()).Elem()
			if err := setScalarValue(elem, value); err != nil {
				return err
			}
			slice = reflect.Append(slice, elem)
		}
		field.Set(slice)
		return nil
	}
	return setScalarValue(field, strings.Join(values, " "))
}

func setScalarValue(field reflect.Value, value string) error {
	if field.CanAddr() {
		if unmarshaler, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return unmarshaler.UnmarshalText([]byte(value))
		}
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil
	case reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("command: cannot bind %q to %s: %w", value, field.Type(), err)
		}
		field.SetBool(parsed)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("command: cannot bind %q to %s: %w", value, field.Type(), err)
		}
		field.SetInt(parsed)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		parsed, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("command: cannot bind %q to %s: %w", value, field.Type(), err)
		}
		field.SetUint(parsed)
		return nil
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("command: cannot bind %q to %s: %w", value, field.Type(), err)
		}
		field.SetFloat(parsed)
		return nil
	default:
		return fmt.Errorf("command: unsupported typed field %s", field.Type())
	}
}
