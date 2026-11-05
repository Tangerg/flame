package plugin

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

// schemaNode is a read-only view of one schema within a method's input or
// output document. It resolves local references and merges allOf members —
// the shapes Go types reflect to — and nothing more; validation of values
// stays with the contract.
type schemaNode struct {
	fields map[string]jsontext.Value
	defs   map[string]jsontext.Value
}

func newSchemaNode(document jsontext.Value) (schemaNode, error) {
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(document, &fields); err != nil {
		return schemaNode{}, fmt.Errorf("schema: %w", err)
	}
	var defs map[string]jsontext.Value
	if raw, ok := fields["$defs"]; ok {
		if err := json.Unmarshal(raw, &defs); err != nil {
			return schemaNode{}, fmt.Errorf("schema definitions: %w", err)
		}
	}
	return schemaNode{fields: fields, defs: defs}.resolved(), nil
}

func (n schemaNode) child(raw jsontext.Value) (schemaNode, bool) {
	var fields map[string]jsontext.Value
	if json.Unmarshal(raw, &fields) != nil {
		return schemaNode{}, false
	}
	return schemaNode{fields: fields, defs: n.defs}.resolved(), true
}

// resolved follows a local $ref and folds allOf members into one view.
func (n schemaNode) resolved() schemaNode {
	for depth := 0; depth < 32; depth++ {
		var ref string
		if raw, ok := n.fields["$ref"]; !ok || json.Unmarshal(raw, &ref) != nil {
			break
		}
		fragment, local := strings.CutPrefix(ref, "#")
		if !local {
			break
		}
		fragment, err := url.PathUnescape(fragment)
		pointer := jsontext.Pointer(fragment)
		if err != nil || !pointer.IsValid() || pointer.Parent() != "/$defs" {
			break
		}
		target, ok := n.defs[pointer.LastToken()]
		if !ok {
			break
		}
		var fields map[string]jsontext.Value
		if json.Unmarshal(target, &fields) != nil {
			break
		}
		n.fields = fields
	}
	var members []jsontext.Value
	if raw, ok := n.fields["allOf"]; ok && json.Unmarshal(raw, &members) == nil {
		merged := make(map[string]jsontext.Value, len(n.fields))
		for key, value := range n.fields {
			if key != "allOf" {
				merged[key] = value
			}
		}
		for _, member := range members {
			if resolved, ok := n.child(member); ok {
				for key, value := range resolved.fields {
					if _, set := merged[key]; !set {
						merged[key] = value
					}
				}
			}
		}
		n.fields = merged
	}
	return n
}

func (n schemaNode) properties() map[string]jsontext.Value {
	var properties map[string]jsontext.Value
	if raw, ok := n.fields["properties"]; ok {
		_ = json.Unmarshal(raw, &properties)
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
		raw, ok := current.properties()[segment]
		if !ok {
			return schemaNode{}, false
		}
		if current, ok = current.child(raw); !ok {
			return schemaNode{}, false
		}
	}
	return current, true
}

func (n schemaNode) items() (schemaNode, bool) {
	raw, ok := n.fields["items"]
	if !ok {
		return schemaNode{}, false
	}
	return n.child(raw)
}

func (n schemaNode) types() []string {
	raw, ok := n.fields["type"]
	if !ok {
		return nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return []string{single}
	}
	var several []string
	_ = json.Unmarshal(raw, &several)
	return slices.DeleteFunc(several, func(value string) bool { return value == "null" })
}

// formKind reports how a person enters this property, or false when it is not
// a scalar a single field can hold.
func (n schemaNode) formKind() (extension.TerminalFormKind, []string, bool) {
	var options []string
	if raw, ok := n.fields["enum"]; ok {
		if json.Unmarshal(raw, &options) != nil || len(options) == 0 {
			return "", nil, false
		}
		return extension.TerminalFormChoice, options, true
	}
	types := n.types()
	if len(types) != 1 {
		return "", nil, false
	}
	switch types[0] {
	case "string":
		return extension.TerminalFormText, nil, true
	case "integer":
		return extension.TerminalFormInteger, nil, true
	case "number":
		return extension.TerminalFormNumber, nil, true
	case "boolean":
		return extension.TerminalFormBoolean, nil, true
	default:
		return "", nil, false
	}
}

func (n schemaNode) required() []string {
	var required []string
	if raw, ok := n.fields["required"]; ok {
		_ = json.Unmarshal(raw, &required)
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
