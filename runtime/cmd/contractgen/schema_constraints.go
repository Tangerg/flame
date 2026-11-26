package main

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/wiregen"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

func restrictAllowedValues(node *wiregen.Schema, owner reflect.Type, sets []dispatch.AllowedValueSet) {
	for _, set := range sets {
		restrictAllowedValueSet(node, owner, set)
	}
}

func restrictAllowedValueSet(node *wiregen.Schema, owner reflect.Type, set dispatch.AllowedValueSet) {
	_, leaf, found := protocol.WireGoPath(owner, set.Field)
	if !found {
		panic(fmt.Sprintf("contractgen: %s has no field %q", owner.Name(), set.Field))
	}
	checkAllowedEnumSubset(owner, set, protocol.WireBaseType(leaf.Type))
	parent, name := descend(node, owner, set.Field, pathMayBeAbsent)
	setAllowedValues(parent, name, owner, set)
}

func checkAllowedEnumSubset(owner reflect.Type, set dispatch.AllowedValueSet, fieldType reflect.Type) {
	values, known := enumValues(fieldType)
	if !known {
		return
	}
	for _, value := range set.Values {
		if !slices.Contains(values, value) {
			panic(fmt.Sprintf(
				"contractgen: %s.%s allows %q outside enum %s",
				owner.Name(), set.Field, value, fieldType.Name(),
			))
		}
	}
}

func setAllowedValues(parent *wiregen.Schema, name string, owner reflect.Type, set dispatch.AllowedValueSet) {
	if parent.Properties == nil {
		parent.Properties = make(map[string]any)
	}
	existing, present := parent.Properties[name]
	if !present {
		parent.Properties[name] = &wiregen.Schema{Enum: slices.Clone(set.Values)}
		return
	}
	narrowed, ok := existing.(*wiregen.Schema)
	if !ok || narrowed.Const != nil || len(narrowed.Enum) != 0 {
		panic(fmt.Sprintf("contractgen: %s.%s has conflicting branch constraints", owner.Name(), set.Field))
	}
	narrowed.Enum = slices.Clone(set.Values)
}

func constrain(node *wiregen.Schema, owner reflect.Type, required, forbid []string) {
	for _, path := range required {
		parent, leaf := descend(node, owner, path, pathMustExist)
		parent.Required = append(parent.Required, leaf)
	}
	for _, path := range forbid {
		parent, leaf := descend(node, owner, path, pathMayBeAbsent)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		parent.Properties[leaf] = false
	}
}

// A path that must be present requires every intermediate object as well.
type pathPresence int

const (
	pathMayBeAbsent pathPresence = iota
	pathMustExist
)

func descend(node *wiregen.Schema, owner reflect.Type, path string, presence pathPresence) (*wiregen.Schema, string) {
	segments := strings.Split(path, ".")
	current := node
	for _, segment := range segments[:len(segments)-1] {
		field, ok := protocol.WireFieldNamed(owner, segment)
		if !ok {
			panic(fmt.Sprintf("contractgen: no JSON field %q on %s", segment, owner))
		}
		owner = protocol.WireBaseType(field.Type)
		if current.Properties == nil {
			current.Properties = make(map[string]any)
		}
		child, ok := current.Properties[segment].(*wiregen.Schema)
		if !ok {
			child = &wiregen.Schema{}
			current.Properties[segment] = child
		}
		if presence == pathMustExist {
			current.Required = append(current.Required, segment)
		}
		current = child
	}
	return current, segments[len(segments)-1]
}

func normalize(node *wiregen.Schema) {
	if node == nil {
		return
	}
	slices.Sort(node.Required)
	node.Required = slices.Compact(node.Required)
	for _, value := range node.Properties {
		if child, ok := value.(*wiregen.Schema); ok {
			normalize(child)
		}
	}
	for _, child := range slices.Concat(node.OneOf, node.AnyOf, node.AllOf) {
		normalize(child)
	}
	normalize(node.Items)
	normalize(node.If)
	normalize(node.Then)
}

func applyValueConstraint(node *wiregen.Schema, constraint dispatch.FieldConstraint) {
	if target, kind, textual := stringConstraintTarget(node, constraint.Kind); textual {
		applyStringConstraint(target, kind, constraint)
		return
	}
	applyShapeConstraint(node, constraint)
}

func stringConstraintTarget(node *wiregen.Schema, kind dispatch.ConstraintKind) (*wiregen.Schema, dispatch.ConstraintKind, bool) {
	switch kind {
	case dispatch.ConstraintNonEmpty, dispatch.ConstraintMaxLength, dispatch.ConstraintIdentity, dispatch.ConstraintPrefix, dispatch.ConstraintPattern:
		return node, kind, true
	case dispatch.ConstraintMaxItemLength:
		return node.Items, dispatch.ConstraintMaxLength, true
	case dispatch.ConstraintIdentityItems:
		return node.Items, dispatch.ConstraintIdentity, true
	case dispatch.ConstraintPatternItems:
		return node.Items, dispatch.ConstraintPattern, true
	case dispatch.ConstraintMaxPropertyNameLength:
		return propertyNames(node), dispatch.ConstraintMaxLength, true
	case dispatch.ConstraintIdentityPropertyNames:
		return propertyNames(node), dispatch.ConstraintIdentity, true
	default:
		return nil, kind, false
	}
}

func applyStringConstraint(target *wiregen.Schema, kind dispatch.ConstraintKind, constraint dispatch.FieldConstraint) {
	switch kind {
	case dispatch.ConstraintNonEmpty:
		target.MinLength = new(1)
	case dispatch.ConstraintMaxLength:
		target.MaxLength = new(int(constraint.Limit))
	case dispatch.ConstraintIdentity:
		addPattern(target, dispatch.IdentityPattern)
	case dispatch.ConstraintPrefix:
		addPattern(target, "^"+regexp.QuoteMeta(constraint.Value))
	case dispatch.ConstraintPattern:
		addPattern(target, constraint.Value)
	case dispatch.ConstraintPositive, dispatch.ConstraintNonNegative, dispatch.ConstraintNonEmptyItems, dispatch.ConstraintNonEmptyProperties, dispatch.ConstraintUniqueItems, dispatch.ConstraintMinItems, dispatch.ConstraintMaxItems, dispatch.ConstraintMaxItemLength, dispatch.ConstraintIdentityItems, dispatch.ConstraintMaxPropertyNameLength, dispatch.ConstraintIdentityPropertyNames, dispatch.ConstraintPatternItems, dispatch.ConstraintMinimum, dispatch.ConstraintMaximum:
	}
}

func applyShapeConstraint(node *wiregen.Schema, constraint dispatch.FieldConstraint) {
	switch constraint.Kind {
	case dispatch.ConstraintPositive:
		if node.Type == wiregen.SchemaTypeNumber {
			node.ExclusiveMinimum = new(int64(0))
		} else {
			node.Minimum = new(int64(1))
		}
	case dispatch.ConstraintNonNegative:
		node.Minimum = new(int64(0))
	case dispatch.ConstraintNonEmptyItems:
		node.MinItems = new(1)
	case dispatch.ConstraintNonEmptyProperties:
		node.MinProperties = new(1)
	case dispatch.ConstraintUniqueItems:
		node.UniqueItems = true
	case dispatch.ConstraintMinItems:
		node.MinItems = new(int(constraint.Limit))
	case dispatch.ConstraintMaxItems:
		node.MaxItems = new(int(constraint.Limit))
	case dispatch.ConstraintMinimum:
		node.Minimum = new(constraint.Limit)
	case dispatch.ConstraintMaximum:
		node.Maximum = new(constraint.Limit)
	default:
		panic(fmt.Sprintf("contractgen: unsupported value constraint %s", constraint.Kind))
	}
}

// JSON Schema has a single pattern keyword, so a second predicate becomes an allOf
// member rather than silently replacing the first.
func addPattern(node *wiregen.Schema, pattern string) {
	if node.Pattern == "" {
		node.Pattern = pattern
		return
	}
	if node.Pattern == pattern || slices.ContainsFunc(node.AllOf, func(member *wiregen.Schema) bool {
		return member.Pattern == pattern
	}) {
		return
	}
	node.AllOf = append(node.AllOf, &wiregen.Schema{Pattern: pattern})
}

func propertyNames(node *wiregen.Schema) *wiregen.Schema {
	if node.PropertyNames == nil {
		node.PropertyNames = &wiregen.Schema{}
	}
	return node.PropertyNames
}
