package main

import (
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/qimu/contractgen"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

// The schema walker turns the registered wire types into JSON Schema.
//
// Reflection alone would produce a schema that says `type: object` for a closed
// union and `type: string` for a three-word enum — technically true, useless as a
// contract, and laxer than the runtime. So the walk reads the SPECS for everything
// reflection cannot see (discriminators, variant field/value sets, conditional rules,
// enum value sets) and reads the Go types for everything it can (field names,
// optionality, nesting). Neither half guesses at the other's job.
//
// What it deliberately does NOT emit on SHARED DEFINITIONS is
// `additionalProperties: false`. A result may grow optional fields and an older
// client must keep accepting it. Requests are different: dispatch strictly
// decodes their DTOs, so OpenRPC closes typed objects throughout each request
// projection while leaving maps, opaque values and reusable result shapes open.
//
// Nor does it widen a required field to accept null. A nil Go slice or map DOES
// marshal to null, so a mechanical reading would publish `T[] | null` on most list
// fields — but `required` already states the contract: the field is there, with a
// value of that type. Publishing the null would push a runtime defect onto every
// client as defensive code, forever. The validator rejects it instead, which is
// where a broken frame should surface.

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[jsontext.Value]()
)

// schemaSet is the walked type graph: one definition per named wire type, plus
// the specs that tell the walk what reflection cannot see.
type schemaSet struct {
	walked map[reflect.Type]*contractgen.Schema
	defs   map[string]*contractgen.Schema
	origin map[string]reflect.Type
	enums  map[reflect.Type][]string

	// embedded names every definition reached by being embedded into another
	// struct; referenced names every definition reached by a standalone $ref — a
	// field, element, variant, result, or request shape. encoding/json inlines an
	// embedded owner's fields into its carrier, so an owner reached ONLY through
	// embedding is never a standalone JSON frame and no document cites it; a type
	// that is also referenced standalone (ModelUsage behind a map, a request a
	// composing request embeds) still is. The walk defines both so carriers inherit
	// field constraints and the Go validator is generated from one graph, but
	// Definitions withholds the embedded-only owners from the published bundle.
	embedded   map[string]bool
	referenced map[string]bool

	unions      map[reflect.Type]dispatch.UnionSpec
	constraints map[reflect.Type][]dispatch.ConditionalRule
	values      map[reflect.Type][]dispatch.FieldConstraint
}

func newSchemaSet(shapes *dispatch.Shapes) *schemaSet {
	set := &schemaSet{
		defs:        make(map[string]*contractgen.Schema),
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

// Definitions returns the definitions the bundle publishes, keyed by name: every
// walked definition except the embedded owners, whose fields are inlined into
// their carriers and which therefore name no standalone JSON frame. Names are
// sorted by the JSON encoder, so the bundle's diff is stable across runs.
func (s *schemaSet) Definitions() map[string]*contractgen.Schema {
	out := make(map[string]*contractgen.Schema, len(s.defs))
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

// walk returns the schema for a Go type, defining it in the bundle if it is a
// named type worth referencing.
func (s *schemaSet) walk(t reflect.Type) (node *contractgen.Schema) {
	defer func() {
		if s.walked == nil {
			s.walked = make(map[reflect.Type]*contractgen.Schema)
		}
		s.walked[t] = node
	}()
	switch t {
	case timeType:
		// RFC 3339 in, RFC 3339 out — time.Time's own JSON encoding.
		return &contractgen.Schema{Type: contractgen.SchemaTypeString, Format: "date-time"}
	case rawMessageType:
		// An opaque passthrough: any JSON value, by design. Naming a shape here
		// would be inventing one the runtime does not enforce.
		return &contractgen.Schema{}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return s.walk(t.Elem())
	case reflect.Interface:
		return &contractgen.Schema{}
	case reflect.Bool:
		return &contractgen.Schema{Type: contractgen.SchemaTypeBoolean}
	case reflect.String:
		if values, ok := s.enums[t]; ok {
			return s.define(t, &contractgen.Schema{Type: contractgen.SchemaTypeString, Enum: slices.Clone(values)})
		}
		if values, ok := enumValues(t); ok {
			return s.define(t, &contractgen.Schema{Type: contractgen.SchemaTypeString, Enum: values})
		}
		return &contractgen.Schema{Type: contractgen.SchemaTypeString}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &contractgen.Schema{Type: contractgen.SchemaTypeInteger}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &contractgen.Schema{Type: contractgen.SchemaTypeInteger, Minimum: new(int64(0))}
	case reflect.Float32, reflect.Float64:
		return &contractgen.Schema{Type: contractgen.SchemaTypeNumber}
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &contractgen.Schema{Type: contractgen.SchemaTypeString, ContentEncoding: "base64"}
		}
		return &contractgen.Schema{Type: contractgen.SchemaTypeArray, Items: s.walk(t.Elem())}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("contractgen: %s has a non-string map key; that is not JSON", t))
		}
		return &contractgen.Schema{Type: contractgen.SchemaTypeObject, AdditionalProps: s.walk(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return s.object(t)
		}
		return s.define(t, nil)
	default:
		panic(fmt.Sprintf("contractgen: %s (%s) has no JSON representation", t, t.Kind()))
	}
}

// define reserves a definition name, then fills it. The name is reserved BEFORE
// the body is walked so a type that reaches itself resolves to a reference
// instead of recursing forever.
func (s *schemaSet) define(t reflect.Type, body *contractgen.Schema) *contractgen.Schema {
	name := defName(t)
	// define is the standalone-reference path: a $ref to this name is being placed
	// into a shape, which keeps the definition in the published bundle even if the
	// same type is also embedded elsewhere.
	s.referenced[name] = true
	if existing, ok := s.origin[name]; ok {
		if existing != t {
			panic(fmt.Sprintf("contractgen: %s and %s would both be published as %q", existing, t, name))
		}
		return &contractgen.Schema{Ref: contractgen.RefPrefix + name}
	}
	s.reserve(t, body)
	return &contractgen.Schema{Ref: contractgen.RefPrefix + name}
}

// defineEmbedded records that an owner is flattened into a carrier and walks it so
// its field types and Go validator are reached, WITHOUT recording a standalone
// reference: JSON inlines the owner's fields, so no frame $refs the owner itself.
func (s *schemaSet) defineEmbedded(t reflect.Type) {
	s.embedded[defName(t)] = true
	if _, ok := s.origin[defName(t)]; ok {
		return
	}
	s.reserve(t, nil)
}

// reserve fills a definition, reserving its name before the body is walked so a
// type that reaches itself resolves to a reference instead of recursing forever.
func (s *schemaSet) reserve(t reflect.Type, body *contractgen.Schema) {
	s.origin[defName(t)] = t
	if body == nil {
		body = s.object(t)
	}
	s.defs[defName(t)] = body
}

// object walks a struct's fields. Optionality starts with the json tag, then an
// unconditional requirement may narrow an omittable field into a required wire
// fact.
func (s *schemaSet) object(t reflect.Type) *contractgen.Schema {
	out := &contractgen.Schema{Type: contractgen.SchemaTypeObject, Properties: make(map[string]any)}
	// Flattened fields still have a public named owner and its wire validator.
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous && field.IsExported() {
			embedded := field.Type
			if embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct && embedded.PkgPath() == t.PkgPath() {
				s.defineEmbedded(embedded)
			}
		}
	}
	for _, field := range jsonvalue.Fields(t) {
		node := s.walk(field.Type)
		// A value constraint on a directly-owned field narrows it in place; the
		// declared constraint and the generated Go check read the same statement.
		if node.Ref == "" {
			applyValueConstraints(node, s.valueConstraintsFor(t, field.Name))
		}
		out.Properties[field.Name] = node
		if !field.Optional {
			out.Required = append(out.Required, field.Name)
		}
	}
	// A constraint on a NESTED field cannot narrow the shared definition it lives in
	// — the rule belongs to this enclosing shape, not to every carrier of that type
	// — so it rides an allOf branch on the owner instead.
	for _, constraint := range s.values[t] {
		if !strings.Contains(constraint.Field, ".") {
			continue
		}
		branch := &contractgen.Schema{}
		parent, leaf := descend(branch, t, constraint.Field, false)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		narrowed := &contractgen.Schema{}
		applyValueConstraints(narrowed, []dispatch.FieldConstraint{constraint})
		parent.Properties[leaf] = narrowed
		out.AllOf = append(out.AllOf, branch)
	}
	if union, ok := s.unions[t]; ok {
		// A variant states the whole frame it permits, which is stricter and more
		// precise than the encoder's per-field view — so the branches own `required`
		// and the base states none.
		out.Required = nil
		out.OneOf = s.variants(t, union)
	}
	for _, rule := range s.constraints[t] {
		if len(rule.When) == 0 && len(rule.RequiredAny) == 0 &&
			len(rule.Forbidden) == 0 && len(rule.AllowedValues) == 0 {
			// A bare requirement is part of the object's base shape. Keeping it in
			// allOf is schema-equivalent, but hides requiredness from generated
			// structural clients such as TypeScript interfaces.
			constrain(out, t, rule.Required, nil)
			continue
		}
		out.AllOf = append(out.AllOf, s.conditional(t, rule))
	}
	normalize(out)
	return out
}

// variants renders a discriminated union as `oneOf`: each branch pins or narrows
// the discriminator, requires that branch's fields, and forbids every field
// belonging to another branch.
func (s *schemaSet) variants(t reflect.Type, union dispatch.UnionSpec) []*contractgen.Schema {
	tagValues := s.discriminatorValues(t, union)
	roots := jsonvalue.FieldNames(t)

	out := make([]*contractgen.Schema, 0, len(union.Variants)+1)
	for _, variant := range union.Variants {
		if tagValues != nil && !slices.Contains(tagValues, variant.Tag) {
			panic(fmt.Sprintf("contractgen: %s variant %q is not a value of its discriminator type", t.Name(), variant.Tag))
		}
		claimed := slices.Concat(variant.Required, variant.Optional)
		branch := &contractgen.Schema{
			Properties: map[string]any{union.Discriminator: &contractgen.Schema{Const: variant.Tag}},
			Required:   []string{union.Discriminator},
		}
		forbiddenFields := append(forbidden(t, union.Discriminator, roots, claimed), union.Forbidden...)
		constrain(branch, t, variant.Required, forbiddenFields)
		restrictAllowedValues(branch, t, variant.AllowedValues)
		normalize(branch)
		out = append(out, branch)
	}
	if pattern := union.PatternVariant; pattern != nil {
		claimed := slices.Concat(pattern.Required, pattern.Optional)
		branch := &contractgen.Schema{
			Properties: map[string]any{union.Discriminator: &contractgen.Schema{
				Type: contractgen.SchemaTypeString, Pattern: pattern.TagPattern,
				TypeScriptType: pattern.TypeScriptType,
			}},
			Required: []string{union.Discriminator},
		}
		forbiddenFields := append(forbidden(t, union.Discriminator, roots, claimed), union.Forbidden...)
		constrain(branch, t, pattern.Required, forbiddenFields)
		normalize(branch)
		out = append(out, branch)
	}
	return out
}

// discriminatorValues returns the closed value set of the discriminator's own Go
// type, so a variant tag that is not one of them fails generation rather than
// producing a branch nothing can satisfy.
func (s *schemaSet) discriminatorValues(t reflect.Type, union dispatch.UnionSpec) []string {
	field, ok := jsonvalue.LookupField(t, union.Discriminator)
	if !ok {
		return nil
	}
	values, ok := enumValues(field.Type)
	if !ok {
		return nil
	}
	return values
}

// forbidden lists the JSON paths a variant must NOT carry: every root field no
// claim mentions, plus — inside a partially claimed nested frame — every field of
// that frame this variant does not claim.
func forbidden(t reflect.Type, discriminator string, roots, claimed []string) []string {
	claimedRoots := []string{discriminator}
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
		field, ok := jsonvalue.LookupField(t, root)
		if !ok {
			continue
		}
		for _, name := range jsonvalue.FieldNames(jsonvalue.Deref(field.Type)) {
			if !slices.Contains(names, name) {
				out = append(out, root+"."+name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// presence renders one cross-field rule as `if/then`, or as a bare requirement
// when the rule is unconditional.
func (s *schemaSet) conditional(t reflect.Type, rule dispatch.ConditionalRule) *contractgen.Schema {
	then := &contractgen.Schema{}
	constrain(then, t, rule.Required, rule.Forbidden)
	for _, field := range rule.RequiredAny {
		alternative := &contractgen.Schema{}
		constrain(alternative, t, []string{field}, nil)
		then.AnyOf = append(then.AnyOf, alternative)
	}
	restrictAllowedValues(then, t, rule.AllowedValues)
	normalize(then)
	if len(rule.When) == 0 {
		return then
	}
	condition := &contractgen.Schema{}
	for _, when := range rule.When {
		parent, leaf := descend(condition, t, when.Field, true)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		// An equals condition pins the value; a presence condition only asks that
		// the field be there, which `required` already says.
		switch when.Operator {
		case delivery.OperatorEquals:
			parent.Properties[leaf] = &contractgen.Schema{Const: when.Value}
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
	return &contractgen.Schema{If: condition, Then: then}
}

func restrictAllowedValues(node *contractgen.Schema, owner reflect.Type, sets []dispatch.AllowedValueSet) {
	for _, set := range sets {
		restrictAllowedValueSet(node, owner, set)
	}
}

func restrictAllowedValueSet(node *contractgen.Schema, owner reflect.Type, set dispatch.AllowedValueSet) {
	_, leaf, found := jsonvalue.GoPath(owner, set.Field)
	if !found {
		panic(fmt.Sprintf("contractgen: %s has no field %q", owner.Name(), set.Field))
	}
	checkAllowedEnumSubset(owner, set, jsonvalue.Deref(leaf.Type))
	parent, name := descend(node, owner, set.Field, false)
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

func setAllowedValues(parent *contractgen.Schema, name string, owner reflect.Type, set dispatch.AllowedValueSet) {
	if parent.Properties == nil {
		parent.Properties = make(map[string]any)
	}
	existing, present := parent.Properties[name]
	if !present {
		parent.Properties[name] = &contractgen.Schema{Enum: slices.Clone(set.Values)}
		return
	}
	narrowed, ok := existing.(*contractgen.Schema)
	if !ok || narrowed.Const != "" || len(narrowed.Enum) != 0 {
		panic(fmt.Sprintf("contractgen: %s.%s has conflicting branch constraints", owner.Name(), set.Field))
	}
	narrowed.Enum = slices.Clone(set.Values)
}

// constrain applies required and forbidden JSON paths to a sub-schema, descending
// into nested frames for a dotted path.
func constrain(node *contractgen.Schema, owner reflect.Type, required, forbid []string) {
	for _, path := range required {
		// A frame that must contain something must itself be present.
		parent, leaf := descend(node, owner, path, true)
		parent.Required = append(parent.Required, leaf)
	}
	for _, path := range forbid {
		// Forbidding a nested field says nothing about whether the frame is there.
		parent, leaf := descend(node, owner, path, false)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		parent.Properties[leaf] = false
	}
}

// descend walks a dotted path, creating the nested sub-schemas it passes through,
// and returns the sub-schema owning the last segment plus that segment's name.
func descend(node *contractgen.Schema, owner reflect.Type, path string, markIntermediate bool) (*contractgen.Schema, string) {
	segments := strings.Split(path, ".")
	current := node
	for _, segment := range segments[:len(segments)-1] {
		field, ok := jsonvalue.LookupField(owner, segment)
		if !ok {
			panic(fmt.Sprintf("contractgen: no JSON field %q on %s", segment, owner))
		}
		owner = jsonvalue.Deref(field.Type)
		if current.Properties == nil {
			current.Properties = make(map[string]any)
		}
		child, ok := current.Properties[segment].(*contractgen.Schema)
		if !ok {
			child = &contractgen.Schema{}
			current.Properties[segment] = child
		}
		if markIntermediate {
			current.Required = append(current.Required, segment)
		}
		current = child
	}
	return current, segments[len(segments)-1]
}

// normalize sorts and dedupes every `required` list in a subtree so the emitted
// artifact is byte-stable — the drift gate compares bytes, so an unstable
// generator would report drift on every run.
func normalize(node *contractgen.Schema) {
	if node == nil {
		return
	}
	slices.Sort(node.Required)
	node.Required = slices.Compact(node.Required)
	for _, value := range node.Properties {
		if child, ok := value.(*contractgen.Schema); ok {
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

// defName is the published name of a Go type. A generic instantiation's
// reflect name carries the full import path of its arguments, which is neither
// readable nor a legal JSON pointer segment, so it becomes `PageOfSession`.
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

// applyValueConstraints states a declared value constraint in JSON Schema terms, so
// the schema and the generated validators refuse the same frames.
func applyValueConstraints(node *contractgen.Schema, constraints []dispatch.FieldConstraint) {
	for _, constraint := range constraints {
		switch constraint.Kind {
		case dispatch.ConstraintNonEmpty:
			node.MinLength = new(1)
		case dispatch.ConstraintPositive:
			if node.Type == contractgen.SchemaTypeNumber {
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
		case dispatch.ConstraintMaxLength:
			node.MaxLength = new(int(constraint.Limit))
		case dispatch.ConstraintMaxItemLength:
			node.Items.MaxLength = new(int(constraint.Limit))
		case dispatch.ConstraintIdentity:
			addPattern(node, dispatch.IdentityPattern)
		case dispatch.ConstraintIdentityItems:
			addPattern(node.Items, dispatch.IdentityPattern)
		case dispatch.ConstraintMaxPropertyNameLength:
			propertyNames(node).MaxLength = new(int(constraint.Limit))
		case dispatch.ConstraintIdentityPropertyNames:
			addPattern(propertyNames(node), dispatch.IdentityPattern)
		case dispatch.ConstraintPrefix:
			addPattern(node, "^"+regexp.QuoteMeta(constraint.Value))
		case dispatch.ConstraintPrefixItems:
			addPattern(node.Items, "^"+regexp.QuoteMeta(constraint.Value))
		case dispatch.ConstraintPatternItems:
			addPattern(node.Items, constraint.Value)
		case dispatch.ConstraintPattern:
			addPattern(node, constraint.Value)
		case dispatch.ConstraintMinimum:
			node.Minimum = new(constraint.Limit)
		case dispatch.ConstraintMaximum:
			node.Maximum = new(constraint.Limit)
		default:
			panic(fmt.Sprintf(
				"contractgen: unsupported value constraint %s",
				constraint.Kind,
			))
		}
	}
}

// addPattern preserves every independently declared string predicate. JSON
// Schema has a single pattern keyword, so a second predicate must become an
// allOf member rather than silently replacing the first one. This matters for
// framed identities: their prefix and printable identity alphabet are separate
// public promises and both must survive into every generated artifact.
func addPattern(node *contractgen.Schema, pattern string) {
	if node.Pattern == "" {
		node.Pattern = pattern
		return
	}
	if node.Pattern == pattern || slices.ContainsFunc(node.AllOf, func(member *contractgen.Schema) bool {
		return member.Pattern == pattern
	}) {
		return
	}
	node.AllOf = append(node.AllOf, &contractgen.Schema{Pattern: pattern})
}

func propertyNames(node *contractgen.Schema) *contractgen.Schema {
	if node.PropertyNames == nil {
		node.PropertyNames = &contractgen.Schema{}
	}
	return node.PropertyNames
}
