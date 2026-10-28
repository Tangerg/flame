package jsonvalue

import (
	"fmt"
	"reflect"
	"strings"
)

// Decoding and generated projections must admit the same field ownership.
// Shadowed fields are refused instead of choosing a competing declaration.
type Field struct {
	Name string
	Type reflect.Type

	GoName   string
	Optional bool
}

func Fields(owner reflect.Type) []Field {
	fields, _, err := declaredFields(owner, make(map[reflect.Type]bool))
	if err != nil {
		panic(err)
	}
	return fields
}

func Embeds(owner reflect.Type) []reflect.Type {
	_, embedded, err := declaredFields(Deref(owner), make(map[reflect.Type]bool))
	if err != nil {
		panic(err)
	}
	return embedded
}

func FieldNames(owner reflect.Type) []string {
	fields := Fields(owner)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.Name)
	}
	return out
}

func LookupField(owner reflect.Type, name string) (Field, bool) {
	for _, field := range Fields(owner) {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

func HasPath(root reflect.Type, path string) error {
	current := root
	for segment := range strings.SplitSeq(path, ".") {
		field, ok := LookupField(current, segment)
		if !ok {
			return fmt.Errorf("no JSON field %q on %s", segment, current.Name())
		}
		current = Deref(field.Type)
	}
	return nil
}

// Callers that project Go expressions need the full path because a pointer or
// collection parent cannot be read by the same direct selector as a value struct.
func PathFields(root reflect.Type, path string) ([]Field, bool) {
	current := root
	var fields []Field
	for segment := range strings.SplitSeq(path, ".") {
		field, found := LookupField(current, segment)
		if !found {
			return nil, false
		}
		fields = append(fields, field)
		current = Deref(field.Type)
	}
	return fields, len(fields) > 0
}

func GoPath(root reflect.Type, path string) (selector string, leaf Field, ok bool) {
	fields, ok := PathFields(root, path)
	if !ok {
		return "", Field{}, false
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field.GoName)
	}
	return strings.Join(parts, "."), fields[len(fields)-1], true
}

func Deref(t reflect.Type) reflect.Type {
	seen := make(map[reflect.Type]bool)
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice) && !seen[t] {
		seen[t] = true
		t = t.Elem()
	}
	return t
}

type wireTagOptions struct {
	embedded bool
	optional bool
}

func wireNameOf(field reflect.StructField) (string, wireTagOptions) {
	tag, tagged := field.Tag.Lookup("json")
	if field.Anonymous && (!tagged || tag == "") {
		return "", wireTagOptions{embedded: true}
	}
	if !field.IsExported() {
		return "", wireTagOptions{}
	}
	name, rest, hasOptions := strings.Cut(tag, ",")
	options := wireTagOptions{}
	for option := range strings.SplitSeq(rest, ",") {
		if option == "omitempty" || option == "omitzero" {
			options.optional = true
		}
	}
	switch {
	case !tagged, name == "":
		return field.Name, options
	case name == "-" && !hasOptions:
		// A bare "-" skips the field; `json:"-,"` names it "-".
		return "", wireTagOptions{}
	default:
		return name, options
	}
}
