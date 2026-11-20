package dispatch

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/runtime/internal/delivery"
)

// The shape contract declares which wire types are closed unions and which
// cross-field and value constraints they carry; the method contract lives in
// the delivery registry.
//
// Go models these unions as FLAT tag-discriminated structs (one `type` field plus
// the optional fields that tag allows), which is the right wire shape but tells a
// reader nothing about which fields go with which tag. Reflection cannot recover
// that — a discriminator is not a Go concept — so it is declared here and CHECKED
// against the struct. The check is the point: a spec that names a field the struct
// does not have, or a struct field no variant accounts for, fails at startup
// instead of silently producing a schema that permits an illegal frame.

// UnionDiscriminator is the JSON field carrying every first-party union's tag.
const UnionDiscriminator = "type"

// UnionSpec declares one tag-discriminated union. Literal
// variants are exact; PatternVariant is its only optional extension seam.
type UnionSpec struct {
	GoType   reflect.Type
	Variants []VariantSpec
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

// ObjectConstraintSpec declares cross-field rules inside one DTO.
// It is deliberately frame-local: an invariant spanning runs, interrupts
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
		for _, embedded := range protocol.WireEmbeds(spec.GoType) {
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
	case !strings.HasPrefix(n.Name, notificationNamespace):
		return fmt.Errorf("notification %q must use the notifications namespace", n.Name)
	}
	return nil
}
