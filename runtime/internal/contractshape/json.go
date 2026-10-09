package contractshape

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/exactjson"
)

// DecodeValue preserves authored shape before typed validation can erase it.
func DecodeValue(raw jsontext.Value, dst any, path string) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && Deref(reflect.TypeOf(dst).Elem()).Kind() != reflect.Interface {
		return fmt.Errorf("%s must be an object, got null", path)
	}
	if err := json.Unmarshal(raw, dst, json.RejectUnknownMembers(true), exactjson.Numbers()); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if err := validateValueShape(raw, reflect.TypeOf(dst).Elem(), ""); err != nil {
		return fmt.Errorf("%s.%w", path, err)
	}
	return nil
}

// ShapeError keeps an authored field address available to protocol error projection.
type ShapeError struct {
	Field  string
	Detail string
}

func (e *ShapeError) Error() string {
	return e.Field + " " + e.Detail
}

// validateValueShape keeps typed decoding aligned with the generated schema.
// Pointers in protocol DTOs represent omission, not nullable JSON fields; the
// standard decoder otherwise collapses both spellings to nil. Opaque JSON
// values remain open and may contain null by contract.
func validateValueShape(raw jsontext.Value, target reflect.Type, path string) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target == reflect.TypeFor[jsontext.Value]() || target.Kind() == reflect.Interface {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return &ShapeError{Field: path, Detail: "must not be null"}
	}
	if reflect.PointerTo(target).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return nil
	}

	switch target.Kind() {
	case reflect.Struct:
		return validateStructShape(raw, target, path)
	case reflect.Slice, reflect.Array:
		if target.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		return validateSequenceShape(raw, target.Elem(), path)
	case reflect.Map:
		return validateMapShape(raw, target.Elem(), path)
	}
	return nil
}

func validateStructShape(raw jsontext.Value, target reflect.Type, path string) error {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	for _, field := range Fields(target) {
		value, present := object[field.Name]
		if !present {
			if !field.Optional {
				return &ShapeError{Field: fieldPath(path, field.Name), Detail: "is required"}
			}
			continue
		}
		if err := validateValueShape(value, field.Type, fieldPath(path, field.Name)); err != nil {
			return err
		}
	}
	return nil
}

func validateSequenceShape(raw jsontext.Value, element reflect.Type, path string) error {
	var values []jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	for index, value := range values {
		if err := validateValueShape(value, element, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateMapShape(raw jsontext.Value, element reflect.Type, path string) error {
	var values map[string]jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if err := validateValueShape(values[key], element, MapPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func fieldPath(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
}

// MapPath quotes property names so a map key cannot alias a nested field path.
func MapPath(parent, key string) string {
	return fmt.Sprintf("%s[%q]", parent, key)
}
