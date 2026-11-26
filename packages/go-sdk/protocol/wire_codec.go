package protocol

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	framevalue "github.com/Tangerg/frame/value"
)

func DecodeWire(encoded []byte, target any) error {
	_, err := decodeWire(encoded, target)
	return err
}

func decodeWire(encoded []byte, target any) (WireMembers, error) {
	destination := reflect.ValueOf(target)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return WireMembers{}, errors.New("protocol: wire target must be a non-nil pointer")
	}
	owner := destination.Type().Elem()
	if err := ValidateWireType(owner); err != nil {
		return WireMembers{}, err
	}
	value := reflect.New(owner)
	if err := json.Unmarshal(encoded, value.Interface(), json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
		return WireMembers{}, err
	}
	members, err := validateWireDocument(value.Elem(), encoded)
	if err != nil {
		return WireMembers{}, err
	}
	destination.Elem().Set(value.Elem())
	return members, nil
}

func validateWireDocument(value reflect.Value, encoded jsontext.Value) (WireMembers, error) {
	members := WireMembers{values: make(map[string]jsontext.Value)}
	if err := members.inspect(encoded, value.Type(), ""); err != nil {
		if violation, ok := errors.AsType[*ConstraintError](err); ok {
			violation.Shape = wireShapeName(value.Type())
		}
		return WireMembers{}, err
	}
	if err := validateWireTree(value, members); err != nil {
		return WireMembers{}, err
	}
	return members, nil
}

type WireMembers struct {
	values map[string]jsontext.Value
}

func (members WireMembers) Has(path string) bool {
	_, present := members.values[path]
	return present
}

func (members WireMembers) StringEquals(path, expected string) bool {
	encoded := members.values[path]
	if encoded.Kind() != jsontext.KindString {
		return false
	}
	var actual string
	return json.Unmarshal(encoded, &actual) == nil && actual == expected
}

func (members WireMembers) IsTrue(path string) bool {
	return members.values[path].Kind() == jsontext.KindTrue
}

func (members WireMembers) inspect(encoded jsontext.Value, owner reflect.Type, path string) error {
	for owner.Kind() == reflect.Pointer {
		owner = owner.Elem()
	}
	if owner.Kind() == reflect.Interface || owner == reflect.TypeFor[jsontext.Value]() {
		return nil
	}
	violation := func(field, detail string) error {
		return &ConstraintError{Fields: []FieldError{{Field: joinWirePath("", field), Detail: detail}}}
	}
	if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return violation(path, "must not be null")
	}
	if wireScalar(owner) {
		return nil
	}
	switch owner.Kind() {
	case reflect.Struct:
		return members.inspectObject(encoded, owner, path)
	case reflect.Slice, reflect.Array:
		if owner.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		return members.inspectArray(encoded, owner.Elem(), path)
	case reflect.Map:
		return members.inspectMap(encoded, owner.Elem(), path)
	}
	return nil
}

func (members WireMembers) inspectArray(encoded jsontext.Value, element reflect.Type, path string) error {
	var values []jsontext.Value
	if err := json.Unmarshal(encoded, &values); err != nil {
		return err
	}
	for index, child := range values {
		if err := members.inspect(child, element, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func (members WireMembers) inspectMap(encoded jsontext.Value, element reflect.Type, path string) error {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &object); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(object)) {
		if err := members.inspect(object[key], element, framevalue.MapPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func (members WireMembers) inspectObject(encoded jsontext.Value, owner reflect.Type, path string) error {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &object); err != nil {
		return err
	}
	for _, field := range WireFields(owner) {
		member := joinWirePath(path, field.Name)
		child, found := object[field.Name]
		if !found {
			if !field.Optional {
				return &ConstraintError{Fields: []FieldError{{Field: member, Detail: "is required"}}}
			}
			continue
		}
		members.values[member] = child
		if err := members.inspect(child, field.Type, member); err != nil {
			return err
		}
	}
	return nil
}
