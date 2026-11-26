package protocol

import (
	json "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strings"

	framevalue "github.com/Tangerg/frame/value"
)

type wireValidator interface {
	validateWire(wireObject) []FieldError
}

func ValidateWireTree(value any) error {
	_, err := InspectWire(value)
	return err
}

// The JSON encoder determines member omission for in-process Go calls too.
// Interface-valued payloads are opaque because their schema has a separate owner.
func InspectWire(value any) (WireMembers, error) {
	root := reflect.ValueOf(value)
	if !root.IsValid() {
		return WireMembers{}, nil
	}
	if err := ValidateWireType(root.Type()); err != nil {
		return WireMembers{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return WireMembers{}, err
	}
	return validateWireDocument(root, encoded)
}

func validateWireTree(root reflect.Value, members WireMembers) error {
	fields := validateWireValue(root, "", members)
	if len(fields) == 0 {
		return nil
	}
	return &ConstraintError{Shape: wireShapeName(root.Type()), Fields: uniqueFieldErrors(fields)}
}

func wireShapeName(owner reflect.Type) string {
	for owner.Kind() == reflect.Pointer {
		owner = owner.Elem()
	}
	if name := owner.Name(); name != "" {
		return name
	}
	return "wire value"
}

func validateWireValue(value reflect.Value, path string, members WireMembers) []FieldError {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() == reflect.Interface || wireScalar(value.Type()) {
		return nil
	}

	object := wireObject{value: value, path: path, members: members}
	var fields []FieldError
	if value.CanInterface() {
		if validator, ok := value.Interface().(wireValidator); ok {
			for _, violation := range validator.validateWire(object) {
				violation.Field = joinWirePath(path, violation.Field)
				fields = append(fields, violation)
			}
		}
	}

	switch value.Kind() {
	case reflect.Struct:
		for _, field := range WireFields(value.Type()) {
			if object.present(field.Name) {
				child := value.FieldByName(field.GoName)
				name := joinWirePath(path, field.Name)
				fields = append(fields, validateWireValue(child, name, members)...)
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			return fields
		}
		for index := range value.Len() {
			name := fmt.Sprintf("%s[%d]", path, index)
			fields = append(fields, validateWireValue(value.Index(index), name, members)...)
		}
	case reflect.Map:
		keys := value.MapKeys()
		slices.SortFunc(keys, func(left, right reflect.Value) int { return strings.Compare(left.String(), right.String()) })
		for _, key := range keys {
			name := framevalue.MapPath(path, key.String())
			fields = append(fields, validateWireValue(value.MapIndex(key), name, members)...)
		}
	case reflect.String:
		if field := enumViolation(value, path); field.Field != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

func enumViolation(value reflect.Value, path string) FieldError {
	values, closed := WireEnumValues(value.Type())
	if !closed || slices.Contains(values, value.String()) {
		return FieldError{}
	}
	return FieldError{Field: joinWirePath(path, ""), Detail: "must be one of " + strings.Join(values, ", ")}
}

type wireObject struct {
	value   reflect.Value
	path    string
	members WireMembers
}

func (o wireObject) present(field string) bool {
	return o.members.Has(joinWirePath(o.path, field))
}

func (o wireObject) field(path string) reflect.Value {
	if !o.present(path) {
		return reflect.Value{}
	}
	current := o.value
	for segment := range strings.SplitSeq(path, ".") {
		for current.Kind() == reflect.Pointer {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return reflect.Value{}
		}
		field, found := WireFieldNamed(current.Type(), segment)
		if !found {
			return reflect.Value{}
		}
		current = current.FieldByName(field.GoName)
	}
	return current
}

func joinWirePath(prefix, field string) string {
	switch {
	case prefix == "" && field == "":
		return "$"
	case prefix == "":
		return field
	case field == "":
		return prefix
	default:
		return prefix + "." + field
	}
}

func uniqueFieldErrors(fields []FieldError) []FieldError {
	seen := make(map[FieldError]bool, len(fields))
	out := make([]FieldError, 0, len(fields))
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

type ConstraintError struct {
	Shape  string
	Fields []FieldError
}

func (c *ConstraintError) Error() string {
	parts := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		path := f.Field
		if c.Shape != "" {
			path = c.Shape + "." + path
		}
		parts = append(parts, path+": "+f.Detail)
	}
	return strings.Join(parts, "; ")
}

func (c *ConstraintError) Enrich(data *ProblemData) {
	data.Errors = append(data.Errors, c.Fields...)
}

type wireViolations []FieldError

func (v *wireViolations) add(violations ...FieldError) {
	for _, violation := range violations {
		if violation.Field != "" {
			*v = append(*v, violation)
		}
	}
}
