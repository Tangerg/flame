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
	destination := reflect.ValueOf(target)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return errors.New("protocol: wire target must be a non-nil pointer")
	}
	owner := destination.Type().Elem()
	if err := ValidateWireType(owner); err != nil {
		return err
	}
	value := reflect.New(owner)
	if err := json.Unmarshal(encoded, value.Interface(), json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
		return err
	}
	if err := validateWireDocument(value.Elem(), encoded); err != nil {
		return err
	}
	destination.Elem().Set(value.Elem())
	return nil
}

func validateWireDocument(value reflect.Value, encoded jsontext.Value) error {
	members := make(wireMembers)
	if err := members.inspect(encoded, value.Type(), ""); err != nil {
		if violation, ok := errors.AsType[*ConstraintError](err); ok {
			violation.Shape = wireShapeName(value.Type())
		}
		return err
	}
	return validateWireTree(value, members)
}

type wireMembers map[string]struct{}

func (members wireMembers) inspect(encoded jsontext.Value, owner reflect.Type, path string) error {
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

func (members wireMembers) inspectArray(encoded jsontext.Value, element reflect.Type, path string) error {
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

func (members wireMembers) inspectMap(encoded jsontext.Value, element reflect.Type, path string) error {
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

func (members wireMembers) inspectObject(encoded jsontext.Value, owner reflect.Type, path string) error {
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
		members[member] = struct{}{}
		if err := members.inspect(child, field.Type, member); err != nil {
			return err
		}
	}
	return nil
}
