package transport

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
	"github.com/Tangerg/flame/runtime/internal/exactjson"
)

// DecodeValue decodes one typed wire value without erasing unknown fields or
// explicit nulls. Both HTTP directions use this boundary before protocol validation.
func DecodeValue(raw jsontext.Value, dst any, path string) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%s must be an object, got null", path)
	}
	if err := json.Unmarshal(raw, dst, json.RejectUnknownMembers(true), exactjson.Numbers()); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return validateValueShape(raw, reflect.TypeOf(dst).Elem(), path, false)
}

// ValidateRequiredFields rejects missing members of a remote response. Inbound
// requests leave missing-value diagnostics to the operation's generated validators;
// a response must preserve every field its producer's schema promises to emit.
func ValidateRequiredFields(raw jsontext.Value, target reflect.Type, path string) error {
	return validateValueShape(raw, target, path, true)
}

// validateValueShape keeps typed decoding aligned with the generated schema.
// Pointers in protocol DTOs represent omission, not nullable JSON fields; the
// standard decoder otherwise collapses both spellings to nil. Opaque JSON
// values remain open and may contain null by contract.
func validateValueShape(raw jsontext.Value, target reflect.Type, path string, complete bool) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target == reflect.TypeFor[jsontext.Value]() || target.Kind() == reflect.Interface {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%s must be omitted instead of null", path)
	}
	if reflect.PointerTo(target).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return nil
	}

	switch target.Kind() {
	case reflect.Struct:
		return validateStructShape(raw, target, path, complete)
	case reflect.Slice, reflect.Array:
		if target.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		return validateSequenceShape(raw, target.Elem(), path, complete)
	case reflect.Map:
		return validateMapShape(raw, target.Elem(), path, complete)
	}
	return nil
}

func validateStructShape(raw jsontext.Value, target reflect.Type, path string, complete bool) error {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	for _, field := range contractshape.Fields(target) {
		value, present := object[field.Name]
		if !present {
			if complete && !field.Optional {
				return fmt.Errorf("%s.%s is required", path, field.Name)
			}
			continue
		}
		if err := validateValueShape(value, field.Type, path+"."+field.Name, complete); err != nil {
			return err
		}
	}
	return nil
}

func validateSequenceShape(raw jsontext.Value, element reflect.Type, path string, complete bool) error {
	var values []jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	for index, value := range values {
		if err := validateValueShape(value, element, fmt.Sprintf("%s[%d]", path, index), complete); err != nil {
			return err
		}
	}
	return nil
}

func validateMapShape(raw jsontext.Value, element reflect.Type, path string, complete bool) error {
	var values map[string]jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if err := validateValueShape(values[key], element, path+"."+key, complete); err != nil {
			return err
		}
	}
	return nil
}
