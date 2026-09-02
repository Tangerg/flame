package dispatch

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

// This file holds the shape half of the contract: which wire types are closed
// unions and which cross-field constraints they carry. The method half is
// contract.go.
//
// Go models these unions as FLAT tag-discriminated structs (one `type` field plus
// the optional fields that tag allows), which is the right wire shape but tells a
// reader nothing about which fields go with which tag. Reflection cannot recover
// that — a discriminator is not a Go concept — so it is declared here and CHECKED
// against the struct. The check is the point: a spec that names a field the struct
// does not have, or a struct field no variant accounts for, fails at startup
// instead of silently producing a schema that permits an illegal frame.

// UnionSpec declares one tag-discriminated union (contract §11.2). Literal
// variants are exact; PatternVariant is its only optional extension seam.
type UnionSpec struct {
	GoType reflect.Type
	// Discriminator is the JSON field carrying the tag. The contract fixes it at
	// `type` for every first-party union, with no exceptions.
	Discriminator string
	Variants      []VariantSpec
	// PatternVariant keeps a union extensible without weakening its known variants
	// to `type: string`. Its tag pattern must be disjoint from every literal tag;
	// TypeScriptType is the corresponding narrow string type emitted by the SDK
	// (for example `plugin:${string}/${string}`).
	PatternVariant *PatternVariantSpec
	// Forbidden names wire members that no variant may carry even though the Go
	// shape no longer has them. This is for protocol-level negative invariants
	// under an otherwise open object envelope, such as rejecting a removed
	// sender-controlled reliability assertion. It never enables decoding an old
	// shape.
	Forbidden []string
}

// PatternVariantSpec is the one namespaced extension branch of an otherwise
// literal-tagged union. Required and Optional have the same whole-frame meaning
// as [VariantSpec].
type PatternVariantSpec struct {
	TagPattern     string
	TypeScriptType string
	Required       []string
	Optional       []string
}

// VariantSpec is one tag of a union and the fields that tag brings. Names are
// JSON field names, dotted for a nested frame (`payload.tool`).
type VariantSpec struct {
	Tag           string
	Required      []string
	Optional      []string
	AllowedValues []AllowedValueSet
}

// AllowedValueSet narrows one string field to the listed values inside a union
// variant or conditional rule. Field is a dotted JSON path. The values are a
// typed set rather than a map so registry order remains explicit and generated
// artifacts stay deterministic.
type AllowedValueSet struct {
	Field  string
	Values []string
}

// ObjectConstraintSpec declares cross-field rules inside ONE DTO (contract
// §11.2). It is deliberately frame-local: an invariant spanning runs, interrupts
// or the store is a transaction concern and is declared in the application ring,
// so nothing here ever needs a repository to decide.
type ObjectConstraintSpec struct {
	GoType reflect.Type
	Rules  []ConditionalRule
}

// ConditionalRule says which fields must be present, which set must contribute
// at least one present field, which fields must be absent, or which fields are
// restricted to a smaller value set when a condition holds. Field names are
// dotted JSON paths.
type ConditionalRule struct {
	When          []delivery.FieldCondition
	Required      []string
	RequiredAny   []string
	Forbidden     []string
	AllowedValues []AllowedValueSet
}

// CarriedSpec declares a wire type the method graph cannot reach.
//
// The artifact walk starts from the registered methods, so a delivery-owned shape
// that rides somewhere else is invisible to it; `params._meta`, for example, is
// stripped before typed decoding. Concrete tool results are published from
// toolset's presentation contracts instead of being restated here.
//
// Carrier says WHERE it rides, in wire terms. A bare list of types would publish
// the shapes without answering the only question a reader has about them.
type CarriedSpec struct {
	Carrier string
	GoType  reflect.Type
}

// NotificationSpec declares one downstream JSON-RPC notification and the exact
// params shape it carries. Notifications are not callable methods, so they belong
// to the wire-shape registry rather than the request router.
type NotificationSpec struct {
	Name       string
	ParamsType reflect.Type
}

// ConstraintKind is a value constraint a field's JSON type does not express.
type ConstraintKind string

// IdentityPattern is the JSON Schema/ECMAScript spelling of the domain's
// printable, non-whitespace identity alphabet. Category C contains control,
// format, surrogate, private-use, and unassigned code points; category Z
// contains every separator. Empty is intentionally accepted for optional wire
// members and is rejected separately where a member is required.
const IdentityPattern = `^[^\p{C}\p{Z}]*$`

const (
	// ConstraintNonEmpty rejects the empty string. A required id whose value is ""
	// names nothing, and every transport and generated client should refuse it in
	// the same place rather than each handler deciding.
	ConstraintNonEmpty ConstraintKind = "nonEmpty"
	// ConstraintPositive rejects zero, negative values, NaN, and infinities. It
	// applies to both integral identities/counts and real-valued limits.
	ConstraintPositive ConstraintKind = "positive"
	// ConstraintNonNegative rejects negative numeric values while preserving zero
	// as the wire spelling of an omitted/unbounded limit.
	ConstraintNonNegative ConstraintKind = "nonNegative"
	// ConstraintNonEmptyItems rejects an empty array. An optional narrowing set
	// already uses absence for "no narrower scope", while a required set names the
	// minimum recovery or transaction unit. An empty third spelling has no useful
	// meaning in either direction.
	ConstraintNonEmptyItems ConstraintKind = "nonEmptyItems"
	// ConstraintNonEmptyProperties rejects an empty object map. Secret-map
	// replacement uses omission to preserve and a clear variant to remove, so an
	// empty set value would be a third, ambiguous spelling of clear.
	ConstraintNonEmptyProperties ConstraintKind = "nonEmptyProperties"
	// ConstraintUniqueItems rejects a repeated element. A filter is a set, and a
	// value listed twice means the caller believes it is asking something a set
	// cannot express.
	ConstraintUniqueItems ConstraintKind = "uniqueItems"
	// ConstraintMinItems rejects an array shorter than FieldConstraint.Limit.
	// Unlike ConstraintNonEmptyItems, its bound is part of the contract rather
	// than the special distinction between omission and an explicitly empty set.
	ConstraintMinItems ConstraintKind = "minItems"
	// ConstraintMaxItems rejects an array longer than FieldConstraint.Limit.
	ConstraintMaxItems ConstraintKind = "maxItems"
	// ConstraintMaxLength rejects a string containing more Unicode code points
	// than FieldConstraint.Limit, matching JSON Schema's length semantics.
	ConstraintMaxLength ConstraintKind = "maxLength"
	// ConstraintMaxItemLength applies the same Unicode code-point ceiling to
	// every string member of an array.
	ConstraintMaxItemLength ConstraintKind = "maxItemLength"
	// ConstraintIdentity rejects whitespace, control, format, private-use and
	// unassigned code points. Empty remains the spelling of an omitted optional
	// identity; requiredness is declared separately with ConstraintNonEmpty.
	ConstraintIdentity ConstraintKind = "identity"
	// ConstraintIdentityItems applies ConstraintIdentity to every string member
	// of an array.
	ConstraintIdentityItems ConstraintKind = "identityItems"
	// ConstraintMaxPropertyNameLength bounds every key of a string-keyed map.
	ConstraintMaxPropertyNameLength ConstraintKind = "maxPropertyNameLength"
	// ConstraintIdentityPropertyNames applies ConstraintIdentity to every key of
	// a string-keyed map.
	ConstraintIdentityPropertyNames ConstraintKind = "identityPropertyNames"
	// ConstraintPrefix rejects a string that does not start with
	// FieldConstraint.Value. It is used for framed opaque identities whose prefix
	// is part of the public wire contract even though the remainder is not parsed.
	ConstraintPrefix ConstraintKind = "prefix"
	// ConstraintPrefixItems applies ConstraintPrefix to every string member of
	// an array whose entries share one framed identity namespace.
	ConstraintPrefixItems ConstraintKind = "prefixItems"
	// ConstraintPatternItems applies one exact regular-expression grammar to
	// every string member of an array.
	ConstraintPatternItems ConstraintKind = "patternItems"
	// ConstraintPattern rejects a string that does not match the exact regular
	// expression in FieldConstraint.Value. It is reserved for public wire
	// identities whose canonical grammar is narrower than prefix + printable
	// text, such as a fixed-width lowercase-hex namespace.
	ConstraintPattern ConstraintKind = "pattern"
	// ConstraintMinimum rejects a number smaller than FieldConstraint.Limit.
	// It is inclusive, matching JSON Schema's minimum keyword.
	ConstraintMinimum ConstraintKind = "minimum"
	// ConstraintMaximum rejects a number greater than FieldConstraint.Limit.
	// It is inclusive, matching JSON Schema's maximum keyword.
	ConstraintMaximum ConstraintKind = "maximum"
)

// Valid reports whether c names one supported field constraint.
func (c ConstraintKind) Valid() bool {
	switch c {
	case ConstraintNonEmpty, ConstraintPositive, ConstraintNonNegative,
		ConstraintNonEmptyItems, ConstraintNonEmptyProperties, ConstraintUniqueItems,
		ConstraintMinItems, ConstraintMaxItems, ConstraintMaxLength, ConstraintMaxItemLength,
		ConstraintIdentity, ConstraintIdentityItems, ConstraintMaxPropertyNameLength,
		ConstraintIdentityPropertyNames, ConstraintPrefix, ConstraintPrefixItems, ConstraintPattern,
		ConstraintPatternItems,
		ConstraintMinimum, ConstraintMaximum:
		return true
	default:
		return false
	}
}

func (c ConstraintKind) String() string {
	if !c.Valid() {
		return fmt.Sprintf("ConstraintKind(%q)", string(c))
	}
	return string(c)
}

// FieldConstraint is one field's value constraint. Field is a dotted JSON path.
type FieldConstraint struct {
	Field string
	Kind  ConstraintKind
	Limit int64
	Value string
}

func (f FieldConstraint) String() string {
	switch f.Kind {
	case ConstraintMinItems, ConstraintMaxItems, ConstraintMaxLength, ConstraintMaxItemLength, ConstraintMaxPropertyNameLength, ConstraintMinimum, ConstraintMaximum:
		return fmt.Sprintf("%s(%d)", f.Kind, f.Limit)
	case ConstraintPrefix, ConstraintPrefixItems, ConstraintPattern, ConstraintPatternItems:
		return fmt.Sprintf("%s(%s)", f.Kind, strconv.Quote(f.Value))
	default:
		return f.Kind.String()
	}
}

// FieldConstraintSpec declares the value constraints of one wire shape.
//
// These are the checks reflection cannot see: that a string must be non-empty,
// that a number must exceed or not fall below zero. Closed-enum membership is NOT declared here —
// the enum's value set is already declared, so the check is derived from it, and
// declaring it twice would let the two disagree.
//
// The declaration is the single source: the Go validator, the TypeScript validator
// and the schema's minLength / minimum are all generated from it, which is what
// makes the three equivalent by construction instead of by a reminder.
type FieldConstraintSpec struct {
	GoType      reflect.Type
	Constraints []FieldConstraint
}

// Shapes is the registered shape contract. It is separate from the method
// registry because a union is not a method: several methods carry the same union,
// and the artifacts generated from it (oneOf + discriminator, if/then) are
// per-type, not per-method.
type Shapes struct {
	unions        []UnionSpec
	constraints   []ObjectConstraintSpec
	carried       []CarriedSpec
	notifications []NotificationSpec
	values        []FieldConstraintSpec
}

func (s *Shapes) Unions() []UnionSpec {
	out := make([]UnionSpec, len(s.unions))
	for index, spec := range s.unions {
		out[index] = cloneUnionSpec(spec)
	}
	return out
}

// Constraints returns every registered rule PLUS the rules a shape inherits by
// embedding another constrained shape.
//
// encoding/json inlines an embedded struct's fields, so a rule about those fields
// is true of the embedding type too — but nothing said so, and a rule registered
// for RunSummary silently stopped applying to the RunRef that carries it. The
// inheritance is READ OFF the Go embedding rather than declared, because the
// embedding is already the declaration: a second statement of "RunRef composes
// RunSummary" could disagree with the struct.
func (s *Shapes) Constraints() []ObjectConstraintSpec {
	byType := make(map[reflect.Type]ObjectConstraintSpec, len(s.constraints))
	for _, spec := range s.constraints {
		byType[spec.GoType] = cloneObjectConstraintSpec(spec)
	}
	out := make([]ObjectConstraintSpec, 0, len(s.constraints))
	for _, stored := range s.constraints {
		spec := cloneObjectConstraintSpec(stored)
		for _, embedded := range contractshape.Embeds(spec.GoType) {
			inherited, ok := byType[embedded]
			if !ok {
				continue
			}
			spec.Rules = append(slices.Clone(inherited.Rules), spec.Rules...)
		}
		out = append(out, spec)
	}
	return out
}
func (s *Shapes) Carried() []CarriedSpec { return slices.Clone(s.carried) }
func (s *Shapes) Notifications() []NotificationSpec {
	return slices.Clone(s.notifications)
}
func (s *Shapes) ValueConstraints() []FieldConstraintSpec {
	out := make([]FieldConstraintSpec, len(s.values))
	for index, spec := range s.values {
		spec.Constraints = slices.Clone(spec.Constraints)
		out[index] = spec
	}
	return out
}

func cloneUnionSpec(spec UnionSpec) UnionSpec {
	spec.Forbidden = slices.Clone(spec.Forbidden)
	spec.Variants = slices.Clone(spec.Variants)
	for index := range spec.Variants {
		spec.Variants[index].Required = slices.Clone(spec.Variants[index].Required)
		spec.Variants[index].Optional = slices.Clone(spec.Variants[index].Optional)
		spec.Variants[index].AllowedValues = cloneAllowedValueSets(spec.Variants[index].AllowedValues)
	}
	if spec.PatternVariant != nil {
		pattern := *spec.PatternVariant
		pattern.Required = slices.Clone(pattern.Required)
		pattern.Optional = slices.Clone(pattern.Optional)
		spec.PatternVariant = &pattern
	}
	return spec
}

func cloneObjectConstraintSpec(spec ObjectConstraintSpec) ObjectConstraintSpec {
	spec.Rules = slices.Clone(spec.Rules)
	for index := range spec.Rules {
		spec.Rules[index].When = slices.Clone(spec.Rules[index].When)
		spec.Rules[index].Required = slices.Clone(spec.Rules[index].Required)
		spec.Rules[index].RequiredAny = slices.Clone(spec.Rules[index].RequiredAny)
		spec.Rules[index].Forbidden = slices.Clone(spec.Rules[index].Forbidden)
		spec.Rules[index].AllowedValues = cloneAllowedValueSets(spec.Rules[index].AllowedValues)
	}
	return spec
}

func cloneAllowedValueSets(sets []AllowedValueSet) []AllowedValueSet {
	out := slices.Clone(sets)
	for index := range out {
		out[index].Values = slices.Clone(out[index].Values)
	}
	return out
}

func (s *Shapes) union(spec UnionSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid union spec: " + err.Error())
	}
	if slices.ContainsFunc(s.unions, func(existing UnionSpec) bool {
		return existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: union spec for %s is registered twice",
			spec.GoType.Name(),
		))
	}
	s.unions = append(s.unions, cloneUnionSpec(spec))
}

func (s *Shapes) constraint(spec ObjectConstraintSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid object constraint spec: " + err.Error())
	}
	if slices.ContainsFunc(s.constraints, func(existing ObjectConstraintSpec) bool {
		return existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: object constraint spec for %s is registered twice",
			spec.GoType.Name(),
		))
	}
	s.constraints = append(s.constraints, cloneObjectConstraintSpec(spec))
}

func (s *Shapes) valueConstraint(spec FieldConstraintSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid value constraint spec: " + err.Error())
	}
	if slices.ContainsFunc(s.values, func(existing FieldConstraintSpec) bool {
		return existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: value constraint spec for %s is registered twice",
			spec.GoType.Name(),
		))
	}
	spec.Constraints = slices.Clone(spec.Constraints)
	s.values = append(s.values, spec)
}

func (s *Shapes) carriedShape(spec CarriedSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid carried shape spec: " + err.Error())
	}
	if slices.ContainsFunc(s.carried, func(existing CarriedSpec) bool {
		return existing.Carrier == spec.Carrier && existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: carried shape %s on %q is registered twice",
			spec.GoType,
			spec.Carrier,
		))
	}
	s.carried = append(s.carried, spec)
}

func (s *Shapes) notification(spec NotificationSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid notification spec: " + err.Error())
	}
	if slices.ContainsFunc(s.notifications, func(existing NotificationSpec) bool {
		return existing.Name == spec.Name
	}) {
		panic(fmt.Sprintf("dispatch: notification %q is registered twice", spec.Name))
	}
	s.notifications = append(s.notifications, spec)
}

// validate checks a union spec against the struct it describes.
func (u UnionSpec) validate() error {
	if u.GoType == nil || u.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("union spec needs a struct type, got %v", u.GoType)
	}
	validation := unionValidation{
		spec: u, name: u.GoType.Name(),
		accounted: []string{u.Discriminator},
		tags:      make(map[string]bool, len(u.Variants)),
	}
	if err := validation.validateDiscriminator(); err != nil {
		return err
	}
	if err := validation.validateForbiddenFields(); err != nil {
		return err
	}
	for _, variant := range u.Variants {
		if err := validation.validateLiteralVariant(variant); err != nil {
			return err
		}
	}
	if err := validation.validatePatternVariant(); err != nil {
		return err
	}
	return validation.validateCoverage()
}

type unionValidation struct {
	spec      UnionSpec
	name      string
	accounted []string
	tags      map[string]bool
}

func (u *unionValidation) validateDiscriminator() error {
	spec := u.spec
	if spec.Discriminator != "type" {
		return fmt.Errorf(
			"%s: discriminator is %q; the contract requires \"type\"",
			u.name,
			spec.Discriminator,
		)
	}
	if err := contractshape.HasPath(spec.GoType, spec.Discriminator); err != nil {
		return fmt.Errorf("%s: %w", u.name, err)
	}
	if len(spec.Variants) == 0 {
		return fmt.Errorf("%s: a union with no literal variants describes nothing", u.name)
	}
	return nil
}

func (u *unionValidation) validateForbiddenFields() error {
	spec := u.spec
	for index, field := range spec.Forbidden {
		switch {
		case field == "":
			return fmt.Errorf("%s: forbidden field %d has no name", u.name, index)
		case strings.Contains(field, "."):
			return fmt.Errorf(
				"%s: forbidden field %q must be a top-level JSON member",
				u.name,
				field,
			)
		case slices.Contains(spec.Forbidden[:index], field):
			return fmt.Errorf("%s: forbidden field %q is declared twice", u.name, field)
		case slices.Contains(contractshape.FieldNames(spec.GoType), field):
			return fmt.Errorf(
				"%s: forbidden field %q still exists on the Go wire shape",
				u.name,
				field,
			)
		}
	}
	return nil
}

func (u *unionValidation) validateLiteralVariant(variant VariantSpec) error {
	if variant.Tag == "" {
		return fmt.Errorf("%s: a variant needs a tag", u.name)
	}
	if u.tags[variant.Tag] {
		return fmt.Errorf("%s: variant %q is declared twice", u.name, variant.Tag)
	}
	u.tags[variant.Tag] = true
	owner := fmt.Sprintf("%s variant %q", u.name, variant.Tag)
	if err := u.claimFields(
		owner,
		variant.Required,
		variant.Optional,
	); err != nil {
		return err
	}
	return validateAllowedValueSets(owner, u.spec.GoType, variant.AllowedValues, nil)
}

func (u *unionValidation) validatePatternVariant() error {
	pattern := u.spec.PatternVariant
	if pattern == nil {
		return nil
	}
	compiled, err := regexp.Compile(pattern.TagPattern)
	switch {
	case pattern.TagPattern == "":
		return fmt.Errorf("%s: pattern variant needs a tag pattern", u.name)
	case err != nil:
		return fmt.Errorf(
			"%s: invalid pattern variant tag %q: %w",
			u.name,
			pattern.TagPattern,
			err,
		)
	case pattern.TypeScriptType == "":
		return fmt.Errorf("%s: pattern variant needs a TypeScript type", u.name)
	}
	for tag := range u.tags {
		if compiled.MatchString(tag) {
			return fmt.Errorf(
				"%s: pattern variant also matches literal tag %q",
				u.name,
				tag,
			)
		}
	}
	return u.claimFields(
		u.name+" pattern variant",
		pattern.Required,
		pattern.Optional,
	)
}

func (u *unionValidation) claimFields(
	owner string,
	required []string,
	optional []string,
) error {
	for index, field := range required {
		if slices.Contains(required[:index], field) {
			return fmt.Errorf("%s: required field %q is declared twice", owner, field)
		}
	}
	for index, field := range optional {
		switch {
		case slices.Contains(optional[:index], field):
			return fmt.Errorf("%s: optional field %q is declared twice", owner, field)
		case slices.Contains(required, field):
			return fmt.Errorf("%s: field %q cannot be both required and optional", owner, field)
		}
	}
	for _, field := range slices.Concat(required, optional) {
		if err := contractshape.HasPath(u.spec.GoType, field); err != nil {
			return fmt.Errorf("%s: %w", owner, err)
		}
		// A nested declaration accounts for the frame that holds it: claiming
		// `payload.tool` claims `payload`.
		root := strings.Split(field, ".")[0]
		if !slices.Contains(u.accounted, root) {
			u.accounted = append(u.accounted, root)
		}
	}
	return nil
}

func (u *unionValidation) validateCoverage() error {
	// The drift that actually happens: a field is added to the struct and no
	// variant claims it, so the generated schema would allow it under every tag.
	for _, field := range contractshape.FieldNames(u.spec.GoType) {
		if !slices.Contains(u.accounted, field) {
			return fmt.Errorf(
				"%s: field %q belongs to no variant — every union field must name its tag",
				u.name,
				field,
			)
		}
	}
	return nil
}

func (o ObjectConstraintSpec) validate() error {
	if o.GoType == nil || o.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("object constraint spec needs a struct type, got %v", o.GoType)
	}
	name := o.GoType.Name()
	if len(o.Rules) == 0 {
		return fmt.Errorf("%s: a constraint spec with no rules constrains nothing", name)
	}
	for index, rule := range o.Rules {
		owner := fmt.Sprintf("%s rule %d", name, index)
		if err := validateRuleConditions(owner, o.GoType, rule.When); err != nil {
			return err
		}
		if err := validateRuleFields(owner, o.GoType, rule); err != nil {
			return err
		}
	}
	return nil
}

func validateRuleFields(owner string, shape reflect.Type, rule ConditionalRule) error {
	if len(rule.Required) == 0 && len(rule.RequiredAny) == 0 && len(rule.Forbidden) == 0 && len(rule.AllowedValues) == 0 {
		return fmt.Errorf("%s: states no field constraint", owner)
	}
	for fieldIndex, field := range rule.Required {
		if slices.Contains(rule.Required[:fieldIndex], field) {
			return fmt.Errorf("%s: required field %q is declared twice", owner, field)
		}
	}
	for fieldIndex, field := range rule.RequiredAny {
		switch {
		case slices.Contains(rule.RequiredAny[:fieldIndex], field):
			return fmt.Errorf("%s: required-any field %q is declared twice", owner, field)
		case slices.Contains(rule.Required, field):
			return fmt.Errorf("%s: field %q cannot be both required and required-any", owner, field)
		}
	}
	for fieldIndex, field := range rule.Forbidden {
		switch {
		case slices.Contains(rule.Forbidden[:fieldIndex], field):
			return fmt.Errorf("%s: forbidden field %q is declared twice", owner, field)
		case slices.Contains(rule.Required, field):
			return fmt.Errorf("%s: field %q cannot be both required and forbidden", owner, field)
		case slices.Contains(rule.RequiredAny, field):
			return fmt.Errorf("%s: field %q cannot be both required-any and forbidden", owner, field)
		}
	}
	for _, field := range slices.Concat(rule.Required, rule.RequiredAny, rule.Forbidden) {
		if err := contractshape.HasPath(shape, field); err != nil {
			return fmt.Errorf("%s: %w", owner, err)
		}
	}
	return validateAllowedValueSets(owner, shape, rule.AllowedValues, rule.Forbidden)
}

func validateRuleConditions(owner string, shape reflect.Type, conditions []delivery.FieldCondition) error {
	for index, condition := range conditions {
		previousConditions := conditions[:index]
		if slices.Contains(previousConditions, condition) {
			return fmt.Errorf(
				"%s: condition for field %q with operator %s is declared twice",
				owner, condition.Field, condition.Operator,
			)
		}
		if err := delivery.ValidateFieldCondition(owner, shape, condition); err != nil {
			return err
		}
		for _, previous := range previousConditions {
			if previous.Field != condition.Field {
				continue
			}
			if previous.Operator == delivery.OperatorEquals && condition.Operator == delivery.OperatorEquals {
				return fmt.Errorf(
					"%s: field %q has conflicting equals conditions %q and %q",
					owner, condition.Field, previous.Value, condition.Value,
				)
			}
			return fmt.Errorf(
				"%s: field %q combines present and equals conditions",
				owner, condition.Field,
			)
		}
	}
	return nil
}

func validateAllowedValueSets(owner string, shape reflect.Type, sets []AllowedValueSet, forbidden []string) error {
	for index, set := range sets {
		if slices.ContainsFunc(sets[:index], func(previous AllowedValueSet) bool {
			return previous.Field == set.Field
		}) {
			return fmt.Errorf("%s: allowed values for field %q are declared twice", owner, set.Field)
		}
		if slices.Contains(forbidden, set.Field) {
			return fmt.Errorf("%s: field %q cannot be both value-restricted and forbidden", owner, set.Field)
		}
		_, leaf, found := contractshape.GoPath(shape, set.Field)
		if !found {
			return fmt.Errorf("%s: %s has no JSON field path %q", owner, shape.Name(), set.Field)
		}
		fieldType := leaf.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() != reflect.String {
			return fmt.Errorf("%s: allowed-values field %q is not a string", owner, set.Field)
		}
		if len(set.Values) == 0 {
			return fmt.Errorf("%s: allowed-values field %q has no values", owner, set.Field)
		}
		for valueIndex, value := range set.Values {
			switch {
			case value == "" || value != strings.TrimSpace(value):
				return fmt.Errorf("%s: allowed value %d for field %q is not canonical", owner, valueIndex, set.Field)
			case slices.Contains(set.Values[:valueIndex], value):
				return fmt.Errorf("%s: allowed value %q for field %q is declared twice", owner, value, set.Field)
			}
		}
	}
	return nil
}

func (f FieldConstraintSpec) validate() error {
	if f.GoType == nil || f.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("value constraint spec needs a struct type, got %v", f.GoType)
	}
	name := f.GoType.Name()
	if len(f.Constraints) == 0 {
		return fmt.Errorf("%s: a constraint spec with no constraints constrains nothing", name)
	}
	for index, constraint := range f.Constraints {
		if slices.ContainsFunc(f.Constraints[:index], func(previous FieldConstraint) bool {
			return previous.Field == constraint.Field && previous.Kind == constraint.Kind
		}) {
			return fmt.Errorf(
				"%s.%s declares constraint %s twice",
				name, constraint.Field, constraint.Kind,
			)
		}
		if err := validateFieldConstraint(name, f.GoType, constraint); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldConstraint(owner string, shape reflect.Type, constraint FieldConstraint) error {
	fields, ok := contractshape.PathFields(shape, constraint.Field)
	if !ok {
		return fmt.Errorf("%s: no JSON field %q", owner, constraint.Field)
	}
	if err := validateConstraintPathProjection(owner, constraint.Field, fields); err != nil {
		return err
	}
	if err := validateConstraintArguments(owner, constraint); err != nil {
		return err
	}
	return validateConstraintTarget(owner, fields[len(fields)-1], constraint)
}

func validateConstraintPathProjection(owner, path string, fields []contractshape.Field) error {
	for _, parent := range fields[:len(fields)-1] {
		if parent.Type.Kind() == reflect.Struct {
			continue
		}
		parentKind := parent.Type.Kind().String()
		if parent.Type.Kind() == reflect.Pointer {
			parentKind = "pointer"
		}
		return fmt.Errorf(
			"%s.%s constraint path has %s parent %q; only value struct parents are supported",
			owner, path, parentKind, parent.Name,
		)
	}
	return nil
}

func validateConstraintArguments(owner string, constraint FieldConstraint) error {
	acceptsLimit := constraint.Kind == ConstraintMinItems ||
		constraint.Kind == ConstraintMaxItems ||
		constraint.Kind == ConstraintMaxLength ||
		constraint.Kind == ConstraintMaxItemLength ||
		constraint.Kind == ConstraintMaxPropertyNameLength ||
		constraint.Kind == ConstraintMinimum ||
		constraint.Kind == ConstraintMaximum
	if acceptsLimit && constraint.Limit <= 0 {
		return fmt.Errorf(
			"%s.%s constraint %s needs a positive limit",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	if !acceptsLimit && constraint.Limit != 0 {
		return fmt.Errorf(
			"%s.%s constraint %s does not accept a limit",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	acceptsValue := constraint.Kind == ConstraintPrefix || constraint.Kind == ConstraintPrefixItems ||
		constraint.Kind == ConstraintPattern || constraint.Kind == ConstraintPatternItems
	if acceptsValue && constraint.Value == "" {
		return fmt.Errorf(
			"%s.%s constraint %s needs a non-empty value",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	if !acceptsValue && constraint.Value != "" {
		return fmt.Errorf(
			"%s.%s constraint %s does not accept a value",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	if constraint.Kind == ConstraintPattern || constraint.Kind == ConstraintPatternItems {
		if _, err := regexp.Compile(constraint.Value); err != nil {
			return fmt.Errorf(
				"%s.%s constraint %s has invalid pattern: %w",
				owner,
				constraint.Field,
				constraint.Kind,
				err,
			)
		}
	}
	return nil
}

func validateConstraintTarget(owner string, field contractshape.Field, constraint FieldConstraint) error {
	declaredType := field.Type
	valueType := declaredType
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	kind := valueType.Kind()
	if err, handled := validateTextualConstraintTarget(owner, valueType, constraint); handled {
		if err != nil {
			return err
		}
		return validateConstraintProjection(owner, field, constraint)
	}
	switch constraint.Kind {
	case ConstraintPositive:
		if kind != reflect.Uint64 && kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can be positive", owner, constraint.Field, kind)
		}
	case ConstraintNonNegative:
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can be non-negative", owner, constraint.Field, kind)
		}
	case ConstraintNonEmptyItems, ConstraintUniqueItems, ConstraintMinItems, ConstraintMaxItems:
		if kind != reflect.Slice {
			return fmt.Errorf("%s.%s is %s; only an array has items", owner, constraint.Field, declaredType.Kind())
		}
	case ConstraintMinimum:
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can have a minimum", owner, constraint.Field, kind)
		}
	case ConstraintMaximum:
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Uint64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can have a maximum", owner, constraint.Field, kind)
		}
	case ConstraintNonEmptyProperties:
		if kind != reflect.Map {
			return fmt.Errorf("%s.%s is %s; only an object map has properties", owner, constraint.Field, declaredType.Kind())
		}
	default:
		return fmt.Errorf(
			"%s.%s has invalid constraint kind %s",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	return validateConstraintProjection(owner, field, constraint)
}

func validateConstraintProjection(owner string, field contractshape.Field, constraint FieldConstraint) error {
	if err := validatePointerConstraintProjection(owner, field, constraint); err != nil {
		return err
	}
	if err := validateConstraintHelperAssignment(owner, field, constraint); err != nil {
		return err
	}
	if constraint.Kind == ConstraintMinItems && !field.Optional {
		return fmt.Errorf(
			"%s.%s constraint %s does not support a required array",
			owner, constraint.Field, constraint.Kind,
		)
	}
	if constraint.Kind == ConstraintUniqueItems {
		return validateUniqueItemsProjection(owner, field, constraint)
	}
	return validateTextualConstraintProjection(owner, field, constraint)
}

func validateConstraintHelperAssignment(owner string, field contractshape.Field, constraint FieldConstraint) error {
	if field.Type.Kind() == reflect.Pointer {
		target := field.Type.Elem()
		if target.Kind() == reflect.String && target != reflect.TypeFor[string]() {
			switch constraint.Kind {
			case ConstraintMaxLength, ConstraintIdentity, ConstraintPrefix, ConstraintPattern:
				return fmt.Errorf(
					"%s.%s constraint %s does not support a named string pointer",
					owner, constraint.Field, constraint.Kind,
				)
			}
		}
		if target.Kind() == reflect.Slice && target.Name() != "" {
			switch constraint.Kind {
			case ConstraintUniqueItems, ConstraintMaxItems, ConstraintMaxItemLength, ConstraintPatternItems:
				return fmt.Errorf(
					"%s.%s constraint %s does not support a named slice pointer",
					owner, constraint.Field, constraint.Kind,
				)
			}
		}
	}
	if constraint.Kind == ConstraintNonEmptyProperties ||
		constraint.Kind == ConstraintMaxPropertyNameLength ||
		constraint.Kind == ConstraintIdentityPropertyNames {
		valueType := field.Type
		if valueType.Kind() == reflect.Pointer {
			valueType = valueType.Elem()
		}
		if valueType.Key() != reflect.TypeFor[string]() {
			return fmt.Errorf(
				"%s.%s constraint %s supports only builtin string keys",
				owner, constraint.Field, constraint.Kind,
			)
		}
	}
	return nil
}

func validatePointerConstraintProjection(owner string, field contractshape.Field, constraint FieldConstraint) error {
	if field.Type.Kind() != reflect.Pointer {
		return nil
	}
	if !field.Optional {
		return fmt.Errorf(
			"%s.%s constraint %s does not support a required pointer field",
			owner, constraint.Field, constraint.Kind,
		)
	}
	switch constraint.Kind {
	case ConstraintNonEmptyItems, ConstraintNonEmptyProperties, ConstraintMinItems,
		ConstraintMaxPropertyNameLength, ConstraintIdentityPropertyNames, ConstraintMinimum:
		return fmt.Errorf(
			"%s.%s constraint %s does not support a pointer target",
			owner, constraint.Field, constraint.Kind,
		)
	default:
		return nil
	}
}

func validateUniqueItemsProjection(owner string, field contractshape.Field, constraint FieldConstraint) error {
	valueType := field.Type
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if !valueType.Elem().Comparable() {
		return fmt.Errorf(
			"%s.%s constraint %s requires comparable items",
			owner, constraint.Field, constraint.Kind,
		)
	}
	return nil
}

func validateTextualConstraintProjection(owner string, field contractshape.Field, constraint FieldConstraint) error {
	declaredKind := field.Type.Kind()
	if constraint.Kind == ConstraintPrefix && declaredKind != reflect.Pointer && field.Optional {
		return fmt.Errorf(
			"%s.%s constraint %s does not support an optional value field",
			owner, constraint.Field, constraint.Kind,
		)
	}
	if (constraint.Kind == ConstraintIdentityItems || constraint.Kind == ConstraintPrefixItems) &&
		declaredKind == reflect.Pointer {
		return fmt.Errorf(
			"%s.%s constraint %s does not support a pointer string array",
			owner, constraint.Field, constraint.Kind,
		)
	}
	return nil
}

func validateTextualConstraintTarget(owner string, valueType reflect.Type, constraint FieldConstraint) (error, bool) {
	kind := valueType.Kind()
	switch {
	case isStringConstraint(constraint.Kind):
		if kind != reflect.String {
			if constraint.Kind == ConstraintMaxLength {
				return fmt.Errorf("%s.%s is %s; only a string has a length", owner, constraint.Field, kind), true
			}
			return fmt.Errorf("%s.%s is %s; only a string can satisfy constraint %s", owner, constraint.Field, kind, constraint.Kind), true
		}
	case isStringItemConstraint(constraint.Kind):
		if kind != reflect.Slice || valueType.Elem().Kind() != reflect.String {
			return fmt.Errorf("%s.%s is %s; constraint %s requires a string array", owner, constraint.Field, valueType, constraint.Kind), true
		}
	case isStringPropertyNameConstraint(constraint.Kind):
		if kind != reflect.Map || valueType.Key().Kind() != reflect.String {
			return fmt.Errorf("%s.%s is %s; constraint %s requires a string-keyed map", owner, constraint.Field, valueType, constraint.Kind), true
		}
	default:
		return nil, false
	}
	return nil, true
}

func isStringConstraint(kind ConstraintKind) bool {
	switch kind {
	case ConstraintNonEmpty, ConstraintMaxLength, ConstraintIdentity, ConstraintPrefix, ConstraintPattern:
		return true
	default:
		return false
	}
}

func isStringItemConstraint(kind ConstraintKind) bool {
	return kind == ConstraintMaxItemLength || kind == ConstraintIdentityItems ||
		kind == ConstraintPrefixItems || kind == ConstraintPatternItems
}

func isStringPropertyNameConstraint(kind ConstraintKind) bool {
	return kind == ConstraintMaxPropertyNameLength || kind == ConstraintIdentityPropertyNames
}

func (c CarriedSpec) validate() error {
	switch {
	case c.Carrier == "":
		return errors.New("carried shape spec needs the wire member it rides in")
	case c.GoType == nil:
		return fmt.Errorf("carrier %q has no type", c.Carrier)
	}
	return nil
}

func (n NotificationSpec) validate() error {
	switch {
	case n.Name == "":
		return errors.New("notification spec needs a method name")
	case n.ParamsType == nil:
		return fmt.Errorf("notification %q has no params type", n.Name)
	case n.ParamsType.Kind() != reflect.Struct || n.ParamsType.Name() == "":
		return fmt.Errorf("notification %q params must be a named struct, got %v", n.Name, n.ParamsType)
	case !strings.HasPrefix(n.Name, "notifications."):
		return fmt.Errorf("notification %q must use the notifications namespace", n.Name)
	}
	return nil
}
