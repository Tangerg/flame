package plugin

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/frame/portable"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const terminalSchemaResource = "urn:flame:terminal-schema"

// Frame has already admitted the callable schema and bounded reference work.
// Presentation reads its conjunctions without assigning precedence to any of them.
type schemaNode struct {
	parts []*jsonschema.Schema
}

func newSchemaNode(document jsontext.Value) (schemaNode, error) {
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return schemaNode{}, fmt.Errorf("terminal schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil)
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(terminalSchemaResource, resource); err != nil {
		return schemaNode{}, fmt.Errorf("terminal schema: %w", err)
	}
	compiled, err := compiler.Compile(terminalSchemaResource)
	if err != nil {
		return schemaNode{}, fmt.Errorf("terminal schema: %w", err)
	}
	return conjoinedSchema(compiled), nil
}

func conjoinedSchema(schemas ...*jsonschema.Schema) schemaNode {
	var node schemaNode
	var collect func(*jsonschema.Schema)
	collect = func(part *jsonschema.Schema) {
		node.parts = append(node.parts, part)
		if part.Ref != nil {
			collect(part.Ref)
		}
		for _, member := range part.AllOf {
			collect(member)
		}
	}
	for _, part := range schemas {
		collect(part)
	}
	return node
}

func (n schemaNode) properties() map[string]schemaNode {
	properties := make(map[string]schemaNode)
	for _, part := range n.parts {
		for name, property := range part.Properties {
			current := properties[name]
			current.parts = append(current.parts, conjoinedSchema(property).parts...)
			properties[name] = current
		}
	}
	return properties
}

func (n schemaNode) property(path string) (schemaNode, bool) {
	segments := splitPath(path)
	if len(segments) == 0 {
		return schemaNode{}, false
	}
	current := n
	for _, segment := range segments {
		var ok bool
		current, ok = current.properties()[segment]
		if !ok {
			return schemaNode{}, false
		}
	}
	return current, true
}

func (n schemaNode) items() (schemaNode, bool) {
	if !n.isType(portable.TypeArray) {
		return schemaNode{}, false
	}
	var items []*jsonschema.Schema
	for _, part := range n.parts {
		if part.Items2020 != nil {
			items = append(items, part.Items2020)
		}
	}
	if len(items) == 0 {
		return schemaNode{}, false
	}
	return conjoinedSchema(items...), true
}

func (n schemaNode) acceptsType(kind portable.SchemaType) bool {
	for _, part := range n.parts {
		if part.Bool != nil && !*part.Bool {
			return false
		}
		if part.Types == nil {
			continue
		}
		allowed := part.Types.ToStrings()
		if slices.Contains(allowed, string(kind)) {
			continue
		}
		if kind == portable.TypeInteger && slices.Contains(allowed, string(portable.TypeNumber)) {
			continue
		}
		return false
	}
	return true
}

func (n schemaNode) types() []portable.SchemaType {
	types := slices.DeleteFunc(portable.ProfileSchema().Types, func(kind portable.SchemaType) bool {
		return kind == portable.TypeNull || !n.acceptsType(kind)
	})
	if slices.Contains(types, portable.TypeNumber) {
		types = slices.DeleteFunc(types, func(kind portable.SchemaType) bool { return kind == portable.TypeInteger })
	}
	return types
}

func (n schemaNode) isType(kind portable.SchemaType) bool {
	return slices.Equal(n.types(), []portable.SchemaType{kind})
}

func (n schemaNode) formKind() (extension.TerminalFormKind, []string, bool) {
	var options []string
	hasEnum := false
	for _, part := range n.parts {
		if part.Enum == nil {
			continue
		}
		var current []string
		for _, value := range part.Enum.Values {
			text, ok := value.(string)
			if !ok {
				return "", nil, false
			}
			current = append(current, text)
		}
		if !hasEnum {
			options, hasEnum = current, true
		} else {
			options = slices.DeleteFunc(options, func(value string) bool { return !slices.Contains(current, value) })
		}
	}
	if hasEnum {
		if len(options) == 0 || !n.acceptsType(portable.TypeString) {
			return "", nil, false
		}
		return extension.TerminalFormChoice, options, true
	}
	types := n.types()
	if len(types) != 1 {
		return "", nil, false
	}
	switch types[0] {
	case portable.TypeString:
		return extension.TerminalFormText, nil, true
	case portable.TypeInteger:
		return extension.TerminalFormInteger, nil, true
	case portable.TypeNumber:
		return extension.TerminalFormNumber, nil, true
	case portable.TypeBoolean:
		return extension.TerminalFormBoolean, nil, true
	default:
		return "", nil, false
	}
}

func (n schemaNode) required() []string {
	var required []string
	for _, part := range n.parts {
		for _, name := range part.Required {
			if !slices.Contains(required, name) {
				required = append(required, name)
			}
		}
	}
	return required
}

// requireBound reports a required property under prefix that no path binds,
// descending into every object bound only through some of its members — a
// partially built object must still be complete.
func (n schemaNode) requireBound(prefix string, bound []string) error {
	join := func(name string) string {
		if prefix == "" {
			return name
		}
		return prefix + "/" + name
	}
	partial := func(path string) bool {
		return slices.ContainsFunc(bound, func(value string) bool { return strings.HasPrefix(value, path+"/") })
	}
	for _, name := range n.required() {
		if path := join(name); !slices.Contains(bound, path) && !partial(path) {
			return fmt.Errorf("required input %q is not bound", path)
		}
	}
	for name := range n.properties() {
		path := join(name)
		if !partial(path) {
			continue
		}
		child, ok := n.property(name)
		if !ok {
			return fmt.Errorf("input %q has no schema", path)
		}
		if err := child.requireBound(path, bound); err != nil {
			return err
		}
	}
	return nil
}
