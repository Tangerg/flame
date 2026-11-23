package main

import (
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/wiregen"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

// Shared definitions deliberately omit additionalProperties: false so a result may
// grow optional fields an older client still accepts. Required fields are never
// widened to accept null: a nil slice marshals to null, but required already
// states the contract and the validator rejects that defect.

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[jsontext.Value]()
)

type schemaSet struct {
	walked map[reflect.Type]*wiregen.Schema
	defs   map[string]*wiregen.Schema
	origin map[string]reflect.Type
	enums  map[reflect.Type][]string

	// encoding/json inlines an embedded owner's fields into its carrier, so an owner
	// reached only through embedding is never a standalone frame and is withheld from
	// the published bundle.
	embedded   map[string]bool
	referenced map[string]bool

	unions      map[reflect.Type]dispatch.UnionSpec
	constraints map[reflect.Type][]dispatch.ConditionalRule
	values      map[reflect.Type][]dispatch.FieldConstraint
}

func newSchemaSet(shapes *dispatch.Shapes) *schemaSet {
	set := &schemaSet{
		defs:        make(map[string]*wiregen.Schema),
		origin:      make(map[string]reflect.Type),
		enums:       make(map[reflect.Type][]string),
		embedded:    make(map[string]bool),
		referenced:  make(map[string]bool),
		unions:      make(map[reflect.Type]dispatch.UnionSpec),
		constraints: make(map[reflect.Type][]dispatch.ConditionalRule),
		values:      make(map[reflect.Type][]dispatch.FieldConstraint),
	}
	for _, union := range shapes.Unions() {
		set.unions[union.GoType] = union
	}
	for _, constraint := range shapes.Constraints() {
		set.constraints[constraint.GoType] = append(set.constraints[constraint.GoType], constraint.Rules...)
	}
	for _, spec := range shapes.ValueConstraints() {
		set.values[spec.GoType] = append(set.values[spec.GoType], spec.Constraints...)
	}
	return set
}

func (s *schemaSet) Definitions() map[string]*wiregen.Schema {
	out := make(map[string]*wiregen.Schema, len(s.defs))
	for name, def := range s.defs {
		if s.embedded[name] && !s.referenced[name] {
			continue
		}
		out[name] = def
	}
	return out
}

func (s *schemaSet) registerEnum(t reflect.Type, values []string) {
	if t.Kind() != reflect.String || t.Name() == "" || len(values) == 0 {
		panic(fmt.Sprintf("contractgen: invalid external enum %s with values %v", t, values))
	}
	if existing, ok := s.enums[t]; ok && !slices.Equal(existing, values) {
		panic(fmt.Sprintf("contractgen: external enum %s has conflicting values %v and %v", t, existing, values))
	}
	s.enums[t] = slices.Clone(values)
}

func (s *schemaSet) walk(t reflect.Type) (node *wiregen.Schema) {
	if _, walked := s.walked[t]; !walked {
		if err := protocol.ValidateWireType(t); err != nil {
			panic(fmt.Sprintf("contractgen: invalid wire type: %v", err))
		}
	}
	return s.walkAdmitted(t, nil)
}

const base64ContentEncoding = "base64"

func (s *schemaSet) stringSchema(t reflect.Type) *wiregen.Schema {
	if values, ok := s.enums[t]; ok {
		return s.define(t, &wiregen.Schema{Type: wiregen.SchemaTypeString, Enum: slices.Clone(values)})
	}
	if values, ok := enumValues(t); ok {
		return s.define(t, &wiregen.Schema{Type: wiregen.SchemaTypeString, Enum: values})
	}
	return &wiregen.Schema{Type: wiregen.SchemaTypeString}
}

func (s *schemaSet) walkAdmitted(t reflect.Type, collections map[reflect.Type]bool) (node *wiregen.Schema) {
	collections, leave := enterCollection(t, collections)
	defer leave()
	defer func() {
		if s.walked == nil {
			s.walked = make(map[reflect.Type]*wiregen.Schema)
		}
		s.walked[t] = node
	}()
	if special, ok := specialSchema(t); ok {
		return special
	}
	if scalar, ok := scalarSchema(t.Kind()); ok {
		return scalar
	}
	switch t.Kind() {
	case reflect.Pointer:
		return s.walkAdmitted(t.Elem(), collections)
	case reflect.Interface:
		return &wiregen.Schema{}
	case reflect.String:
		return s.stringSchema(t)
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &wiregen.Schema{Type: wiregen.SchemaTypeString, ContentEncoding: base64ContentEncoding}
		}
		return &wiregen.Schema{Type: wiregen.SchemaTypeArray, Items: s.walkAdmitted(t.Elem(), collections)}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("contractgen: %s has a non-string map key; that is not JSON", t))
		}
		return &wiregen.Schema{Type: wiregen.SchemaTypeObject, AdditionalProps: s.walkAdmitted(t.Elem(), collections)}
	case reflect.Struct:
		if t.Name() == "" {
			return s.object(t, collections)
		}
		return s.define(t, nil)
	default:
		panic(fmt.Sprintf("contractgen: %s (%s) has no JSON representation", t, t.Kind()))
	}
}

func enterCollection(t reflect.Type, collections map[reflect.Type]bool) (map[reflect.Type]bool, func()) {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
	default:
		return collections, func() {}
	}
	if collections[t] {
		panic(fmt.Sprintf("contractgen: recursive collection %s requires a named object reference", t))
	}
	if collections == nil {
		collections = make(map[reflect.Type]bool)
	}
	collections[t] = true
	return collections, func() { delete(collections, t) }
}

func specialSchema(t reflect.Type) (*wiregen.Schema, bool) {
	switch t {
	case timeType:
		return &wiregen.Schema{Type: wiregen.SchemaTypeString, Format: "date-time"}, true
	case rawMessageType:
		return &wiregen.Schema{}, true
	default:
		return nil, false
	}
}

func scalarSchema(kind reflect.Kind) (*wiregen.Schema, bool) {
	switch kind {
	case reflect.Bool:
		return &wiregen.Schema{Type: wiregen.SchemaTypeBoolean}, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &wiregen.Schema{Type: wiregen.SchemaTypeInteger}, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &wiregen.Schema{Type: wiregen.SchemaTypeInteger, Minimum: new(int64(0))}, true
	case reflect.Float32, reflect.Float64:
		return &wiregen.Schema{Type: wiregen.SchemaTypeNumber}, true
	default:
		return nil, false
	}
}

// The name is reserved before the body is walked so a type that reaches itself
// resolves to a reference instead of recursing forever.
func (s *schemaSet) define(t reflect.Type, body *wiregen.Schema) *wiregen.Schema {
	name := defName(t)
	s.referenced[name] = true
	if existing, ok := s.origin[name]; ok {
		if existing != t {
			panic(fmt.Sprintf("contractgen: %s and %s would both be published as %q", existing, t, name))
		}
		return &wiregen.Schema{Ref: wiregen.RefPrefix + name}
	}
	s.reserve(t, body)
	return &wiregen.Schema{Ref: wiregen.RefPrefix + name}
}

func (s *schemaSet) defineEmbedded(t reflect.Type) {
	s.embedded[defName(t)] = true
	if _, ok := s.origin[defName(t)]; ok {
		return
	}
	s.reserve(t, nil)
}

func (s *schemaSet) reserve(t reflect.Type, body *wiregen.Schema) {
	s.origin[defName(t)] = t
	if body == nil {
		body = s.object(t, nil)
	}
	s.defs[defName(t)] = body
}

func (s *schemaSet) object(t reflect.Type, collections map[reflect.Type]bool) *wiregen.Schema {
	out := &wiregen.Schema{Type: wiregen.SchemaTypeObject, Properties: make(map[string]any)}
	s.defineEmbeddedOwners(t)
	for _, field := range protocol.WireFields(t) {
		node := s.walkAdmitted(field.Type, collections)
		if node.Ref == "" {
			applyValueConstraints(node, s.valueConstraintsFor(t, field.Name))
		}
		out.Properties[field.Name] = node
		if !field.Optional {
			out.Required = append(out.Required, field.Name)
		}
	}
	out.AllOf = append(out.AllOf, s.nestedConstraintBranches(t)...)
	if union, ok := s.unions[t]; ok {
		out.Required = nil
		out.OneOf = s.variants(t, union)
	}
	for _, rule := range s.constraints[t] {
		if len(rule.When) == 0 && len(rule.RequiredAny) == 0 &&
			len(rule.Forbidden) == 0 && len(rule.AllowedValues) == 0 {
			// Keeping a bare requirement in allOf would hide requiredness from generated
			// structural clients such as TypeScript interfaces.
			constrain(out, t, rule.Required, nil)
			continue
		}
		out.AllOf = append(out.AllOf, s.conditional(t, rule))
	}
	normalize(out)
	return out
}

func (s *schemaSet) defineEmbeddedOwners(t reflect.Type) {
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.Anonymous || !field.IsExported() {
			continue
		}
		embedded := field.Type
		if embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if embedded.Kind() == reflect.Struct && embedded.PkgPath() == t.PkgPath() {
			s.defineEmbedded(embedded)
		}
	}
}

func (s *schemaSet) nestedConstraintBranches(t reflect.Type) []*wiregen.Schema {
	var branches []*wiregen.Schema
	for _, constraint := range s.values[t] {
		if !strings.Contains(constraint.Field, ".") {
			continue
		}
		branch := &wiregen.Schema{}
		parent, leaf := descend(branch, t, constraint.Field, pathMayBeAbsent)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		narrowed := &wiregen.Schema{}
		applyValueConstraints(narrowed, []dispatch.FieldConstraint{constraint})
		parent.Properties[leaf] = narrowed
		branches = append(branches, branch)
	}
	return branches
}

func (s *schemaSet) variants(t reflect.Type, union dispatch.UnionSpec) []*wiregen.Schema {
	tagValues := discriminatorValues(t)
	roots := protocol.WireFieldNames(t)

	out := make([]*wiregen.Schema, 0, len(union.Variants)+1)
	for _, variant := range union.Variants {
		if tagValues != nil && !slices.Contains(tagValues, variant.Tag) {
			panic(fmt.Sprintf("contractgen: %s variant %q is not a value of its discriminator type", t.Name(), variant.Tag))
		}
		claimed := slices.Concat(variant.Required, variant.Optional)
		branch := &wiregen.Schema{
			Properties: map[string]any{dispatch.UnionDiscriminator: &wiregen.Schema{Const: variant.Tag}},
			Required:   []string{dispatch.UnionDiscriminator},
		}
		forbiddenFields := append(forbidden(t, roots, claimed), union.Forbidden...)
		constrain(branch, t, variant.Required, forbiddenFields)
		restrictAllowedValues(branch, t, variant.AllowedValues)
		normalize(branch)
		out = append(out, branch)
	}
	if pattern := union.PatternVariant; pattern != nil {
		claimed := slices.Concat(pattern.Required, pattern.Optional)
		branch := &wiregen.Schema{
			Properties: map[string]any{dispatch.UnionDiscriminator: &wiregen.Schema{
				Type: wiregen.SchemaTypeString, Pattern: pattern.TagPattern,
				TypeScriptType: pattern.TypeScriptType,
			}},
			Required: []string{dispatch.UnionDiscriminator},
		}
		forbiddenFields := append(forbidden(t, roots, claimed), union.Forbidden...)
		constrain(branch, t, pattern.Required, forbiddenFields)
		normalize(branch)
		out = append(out, branch)
	}
	return out
}

func discriminatorValues(t reflect.Type) []string {
	field, ok := protocol.WireFieldNamed(t, dispatch.UnionDiscriminator)
	if !ok {
		return nil
	}
	values, ok := enumValues(field.Type)
	if !ok {
		return nil
	}
	return values
}

func forbidden(t reflect.Type, roots, claimed []string) []string {
	claimedRoots := []string{dispatch.UnionDiscriminator}
	nested := make(map[string][]string)
	for _, path := range claimed {
		root, rest, dotted := strings.Cut(path, ".")
		claimedRoots = append(claimedRoots, root)
		if dotted {
			nested[root] = append(nested[root], rest)
		}
	}

	var out []string
	for _, root := range roots {
		if !slices.Contains(claimedRoots, root) {
			out = append(out, root)
		}
	}
	for root, names := range nested {
		field, ok := protocol.WireFieldNamed(t, root)
		if !ok {
			continue
		}
		for _, name := range protocol.WireFieldNames(protocol.WireBaseType(field.Type)) {
			if !slices.Contains(names, name) {
				out = append(out, root+"."+name)
			}
		}
	}
	slices.Sort(out)
	return out
}

func (s *schemaSet) conditional(t reflect.Type, rule dispatch.ConditionalRule) *wiregen.Schema {
	then := &wiregen.Schema{}
	constrain(then, t, rule.Required, rule.Forbidden)
	for _, field := range rule.RequiredAny {
		alternative := &wiregen.Schema{}
		constrain(alternative, t, []string{field}, nil)
		then.AnyOf = append(then.AnyOf, alternative)
	}
	restrictAllowedValues(then, t, rule.AllowedValues)
	normalize(then)
	if len(rule.When) == 0 {
		return then
	}
	condition := &wiregen.Schema{}
	for _, when := range rule.When {
		parent, leaf := descend(condition, t, when.Field, pathMustExist)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		switch when.Operator {
		case delivery.OperatorEquals:
			parent.Properties[leaf] = &wiregen.Schema{Const: when.Value}
		case delivery.OperatorPresent:
		default:
			panic(fmt.Sprintf(
				"contractgen: %s.%s uses unsupported condition operator %s",
				t.Name(),
				when.Field,
				when.Operator,
			))
		}
		parent.Required = append(parent.Required, leaf)
	}
	normalize(condition)
	return &wiregen.Schema{If: condition, Then: then}
}

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
	if !ok || narrowed.Const != "" || len(narrowed.Enum) != 0 {
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

func defName(t reflect.Type) string {
	genericName, arguments, generic := strings.Cut(t.Name(), "[")
	if !generic {
		return genericName
	}
	var out strings.Builder
	out.WriteString(genericName)
	for argument := range strings.SplitSeq(strings.TrimSuffix(arguments, "]"), ",") {
		argument = strings.TrimPrefix(strings.TrimSpace(argument), "*")
		if index := strings.LastIndex(argument, "."); index >= 0 {
			argument = argument[index+1:]
		}
		out.WriteString("Of")
		out.WriteString(argument)
	}
	return out.String()
}

func applyValueConstraints(node *wiregen.Schema, constraints []dispatch.FieldConstraint) {
	for _, constraint := range constraints {
		if target, kind, textual := stringConstraintTarget(node, constraint.Kind); textual {
			applyStringConstraint(target, kind, constraint)
			continue
		}
		applyShapeConstraint(node, constraint)
	}
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
