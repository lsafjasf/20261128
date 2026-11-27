package report

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
)

// JSON marshals a report after replacing non-finite float values with null.
// Go's encoder rejects NaN/Inf, and silently changing them to zero would turn
// "unavailable" into a plausible-looking measurement.
func JSON(v any) ([]byte, error) {
	safe := jsonSafe(reflect.ValueOf(v))
	return json.MarshalIndent(safe, "", "  ")
}

func jsonSafe(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return jsonSafe(v.Elem())
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f
	case reflect.Struct:
		out := make(map[string]any)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name, opts := jsonName(field)
			if name == "-" {
				continue
			}
			fv := v.Field(i)
			if opts["omitempty"] && isEmpty(fv) {
				continue
			}
			out[name] = jsonSafe(fv)
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = jsonSafe(v.Index(i))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = jsonSafe(iter.Value())
		}
		return out
	default:
		return v.Interface()
	}
}

func jsonName(field reflect.StructField) (string, map[string]bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name, nil
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	opts := make(map[string]bool, len(parts)-1)
	for _, opt := range parts[1:] {
		opts[opt] = true
	}
	return name, opts
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}
