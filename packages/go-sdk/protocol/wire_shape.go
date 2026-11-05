package protocol

import (
	"bytes"
	"encoding"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	framevalue "github.com/Tangerg/frame/value"
)

// WireField projects a Runtime DTO declaration. Runtime's protocol includes
// timestamps and finite floating point measurements; portable plugin schemas
// are independently owned by Frame and have a different numeric profile.
type WireField struct {
	Name     string
	Type     reflect.Type
	GoName   string
	Optional bool
}

func WireBaseType(t reflect.Type) reflect.Type {
	seen := make(map[reflect.Type]bool)
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice) && !seen[t] {
		seen[t] = true
		t = t.Elem()
	}
	return t
}

func WireFields(owner reflect.Type) []WireField {
	fields, _, err := wireDeclaration(WireBaseType(owner), make(map[reflect.Type]bool))
	if err != nil {
		panic(err)
	}
	return fields
}

func WireEmbeds(owner reflect.Type) []reflect.Type {
	_, embedded, err := wireDeclaration(WireBaseType(owner), make(map[reflect.Type]bool))
	if err != nil {
		panic(err)
	}
	return embedded
}

func WireFieldNames(owner reflect.Type) []string {
	fields := WireFields(owner)
	names := make([]string, len(fields))
	for index, field := range fields {
		names[index] = field.Name
	}
	return names
}

func WireFieldNamed(owner reflect.Type, name string) (WireField, bool) {
	for _, field := range WireFields(owner) {
		if field.Name == name {
			return field, true
		}
	}
	return WireField{}, false
}

func WirePathFields(owner reflect.Type, path string) ([]WireField, bool) {
	var fields []WireField
	for segment := range strings.SplitSeq(path, ".") {
		field, found := WireFieldNamed(owner, segment)
		if !found {
			return nil, false
		}
		fields = append(fields, field)
		owner = WireBaseType(field.Type)
	}
	return fields, len(fields) != 0
}

func WireGoPath(owner reflect.Type, path string) (string, WireField, bool) {
	fields, found := WirePathFields(owner, path)
	if !found {
		return "", WireField{}, false
	}
	selectors := make([]string, len(fields))
	for index, field := range fields {
		selectors[index] = field.GoName
	}
	return strings.Join(selectors, "."), fields[len(fields)-1], true
}

func ValidateWirePath(owner reflect.Type, path string) error {
	if _, found := WirePathFields(owner, path); !found {
		return fmt.Errorf("protocol: no JSON field %q on %s", path, owner)
	}
	return nil
}

func ValidateWireType(owner reflect.Type) error {
	return validateWireType(owner, make(map[reflect.Type]bool))
}

func validateWireType(owner reflect.Type, visited map[reflect.Type]bool) error {
	if owner == nil {
		return errors.New("protocol: wire type is required")
	}
	pointers := make(map[reflect.Type]bool)
	for owner.Kind() == reflect.Pointer {
		if pointers[owner] {
			return fmt.Errorf("protocol: pointer cycle %s has no wire shape", owner)
		}
		pointers[owner] = true
		owner = owner.Elem()
	}
	if visited[owner] || wireScalar(owner) || owner.Kind() == reflect.Interface {
		return nil
	}
	visited[owner] = true
	if wireCustomCodec(owner) {
		return fmt.Errorf("protocol: %s requires an explicit wire projection", owner)
	}
	switch owner.Kind() {
	case reflect.Slice, reflect.Array:
		return validateWireType(owner.Elem(), visited)
	case reflect.Map:
		if owner.Key().Kind() != reflect.String {
			return fmt.Errorf("protocol: %s requires string object keys", owner)
		}
		return validateWireType(owner.Elem(), visited)
	case reflect.Struct:
		fields, _, err := wireDeclaration(owner, make(map[reflect.Type]bool))
		if err != nil {
			return err
		}
		for _, field := range fields {
			if err := validateWireType(field.Type, visited); err != nil {
				return fmt.Errorf("protocol field %s.%s: %w", owner, field.Name, err)
			}
		}
		return nil
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return nil
	default:
		return fmt.Errorf("protocol: unsupported wire type %s", owner)
	}
}

func wireScalar(owner reflect.Type) bool {
	return owner == reflect.TypeFor[time.Time]() || owner == reflect.TypeFor[jsontext.Value]()
}

func wireCustomCodec(owner reflect.Type) bool {
	for _, candidate := range []reflect.Type{owner, reflect.PointerTo(owner)} {
		if candidate.Implements(reflect.TypeFor[json.Marshaler]()) ||
			candidate.Implements(reflect.TypeFor[json.MarshalerTo]()) ||
			candidate.Implements(reflect.TypeFor[json.Unmarshaler]()) ||
			candidate.Implements(reflect.TypeFor[json.UnmarshalerFrom]()) ||
			candidate.Implements(reflect.TypeFor[encoding.TextMarshaler]()) ||
			candidate.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			return true
		}
	}
	return false
}

func wireDeclaration(owner reflect.Type, visiting map[reflect.Type]bool) ([]WireField, []reflect.Type, error) {
	if owner == nil || owner.Kind() != reflect.Struct || wireScalar(owner) {
		return nil, nil, nil
	}
	if visiting[owner] {
		return nil, nil, fmt.Errorf("protocol: recursive embedding of %s", owner)
	}
	visiting[owner] = true
	defer delete(visiting, owner)
	var fields []WireField
	var embedded []reflect.Type
	for field := range owner.Fields() {
		declared, nested, err := wireMember(owner, field, visiting)
		if err != nil {
			return nil, nil, err
		}
		fields = append(fields, declared...)
		embedded = append(embedded, nested...)
	}
	names, selectors := make(map[string]bool), make(map[string]bool)
	for _, field := range fields {
		if names[field.Name] || selectors[field.GoName] {
			return nil, nil, fmt.Errorf("protocol: ambiguous JSON field %s.%s", owner, field.Name)
		}
		names[field.Name], selectors[field.GoName] = true, true
	}
	return fields, embedded, nil
}

func wireMember(owner reflect.Type, field reflect.StructField, visiting map[reflect.Type]bool) ([]WireField, []reflect.Type, error) {
	tag, tagged := field.Tag.Lookup("json")
	if tag == "-" {
		return nil, nil, nil
	}
	name, options, hasOptions := strings.Cut(tag, ",")
	inline := field.Anonymous && name == ""
	if !field.IsExported() && !inline {
		if tagged {
			return nil, nil, fmt.Errorf("protocol: unexported field %s.%s has a JSON tag", owner, field.Name)
		}
		return nil, nil, nil
	}
	if !utf8.ValidString(name) || strings.ContainsAny(name, "\\'\"`") {
		return nil, nil, fmt.Errorf("protocol: invalid JSON field %s.%s", owner, field.Name)
	}
	if err := validateWireOptions(owner, field.Name, options, hasOptions, inline); err != nil {
		return nil, nil, err
	}
	if !inline {
		if name == "" {
			name = field.Name
		}
		return []WireField{{Name: name, Type: field.Type, GoName: field.Name, Optional: options != ""}}, nil, nil
	}
	base := field.Type
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct || wireCustomCodec(base) {
		return nil, nil, fmt.Errorf("protocol: embedded %s.%s must be a plain struct", owner, field.Name)
	}
	children, nested, err := wireDeclaration(base, visiting)
	if err != nil {
		return nil, nil, err
	}
	return children, append([]reflect.Type{base}, nested...), nil
}

func validateWireOptions(owner reflect.Type, field, options string, present, inline bool) error {
	if !present {
		return nil
	}
	if options == "" {
		return fmt.Errorf("protocol: empty JSON option on %s.%s", owner, field)
	}
	seen := make(map[string]bool)
	for option := range strings.SplitSeq(options, ",") {
		if (option != "omitempty" && option != "omitzero") || seen[option] || inline {
			return fmt.Errorf("protocol: unsupported JSON option %q on %s.%s", option, owner, field)
		}
		seen[option] = true
	}
	return nil
}

// DecodeWire applies the Runtime declaration's presence rules before publishing
// a typed value. Opaque provider payloads keep Frame's exact JSON numbers.
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
	if err := validateWirePresence(encoded, owner, ""); err != nil {
		return err
	}
	destination.Elem().Set(value.Elem())
	return nil
}

func validateWirePresence(encoded jsontext.Value, owner reflect.Type, path string) error {
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
		return validateWireObjectPresence(encoded, owner, path)
	case reflect.Slice, reflect.Array:
		if owner.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var values []jsontext.Value
		if err := json.Unmarshal(encoded, &values); err != nil {
			return err
		}
		for index, child := range values {
			if err := validateWirePresence(child, owner.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		var object map[string]jsontext.Value
		if err := json.Unmarshal(encoded, &object); err != nil {
			return err
		}
		for _, key := range slices.Sorted(maps.Keys(object)) {
			if err := validateWirePresence(object[key], owner.Elem(), framevalue.MapPath(path, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateWireObjectPresence(encoded jsontext.Value, owner reflect.Type, path string) error {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &object); err != nil {
		return err
	}
	for _, field := range WireFields(owner) {
		child, found := object[field.Name]
		if !found {
			if !field.Optional {
				return &ConstraintError{Fields: []FieldError{{Field: joinWirePath("", joinWirePath(path, field.Name)), Detail: "is required"}}}
			}
			continue
		}
		if err := validateWirePresence(child, field.Type, joinWirePath(path, field.Name)); err != nil {
			return err
		}
	}
	return nil
}
