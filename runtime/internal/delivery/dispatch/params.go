package dispatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
)

func decodeParams(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("params must be an object, got null")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("params must contain exactly one JSON object")
	}
	if err := rejectExplicitNulls(raw, reflect.TypeOf(dst).Elem(), "params"); err != nil {
		return err
	}
	return nil
}

// rejectExplicitNulls keeps typed decoding aligned with the generated schema.
// Pointers in protocol DTOs represent omission, not nullable JSON fields; the
// standard decoder otherwise collapses both spellings to nil. Opaque JSON
// values remain open and may contain null by contract.
func rejectExplicitNulls(raw json.RawMessage, target reflect.Type, path string) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target == reflect.TypeFor[json.RawMessage]() || target.Kind() == reflect.Interface {
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
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		for _, field := range contractshape.Fields(target) {
			value, present := object[field.Name]
			if !present {
				continue
			}
			if err := rejectExplicitNulls(value, field.Type, path+"."+field.Name); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if target.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		for index, value := range values {
			if err := rejectExplicitNulls(value, target.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		for _, key := range slices.Sorted(maps.Keys(values)) {
			value := values[key]
			if err := rejectExplicitNulls(value, target.Elem(), path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}
