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

// Go models these unions as flat tag-discriminated structs, and reflection cannot
// recover which fields belong to which tag, so the spec is declared and checked
// against the struct at startup.

const UnionDiscriminator = "type"

type UnionSpec struct {
	GoType         reflect.Type
	Variants       []VariantSpec
	PatternVariant *PatternVariantSpec
	Forbidden      []string
}

type PatternVariantSpec struct {
	TagPattern     string
	TypeScriptType string
	Required       []string
	Optional       []string
}

type VariantSpec struct {
	Tag           string
	Required      []string
	Optional      []string
	AllowedValues []AllowedValueSet
}

type AllowedValueSet struct {
	Field  string
	Values []string
}

type ObjectConstraintSpec struct {
	GoType reflect.Type
	Rules  []ConditionalRule
}

type ConditionalRule struct {
	When          []delivery.FieldCondition
	Required      []string
	RequiredAny   []string
	Forbidden     []string
	AllowedValues []AllowedValueSet
}

type CarriedSpec struct {
	Carrier string
	GoType  reflect.Type
}

type NotificationSpec struct {
	Name       string
	ParamsType reflect.Type
}

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

// encoding/json inlines embedded fields, so an embedded shape's rules apply to the
// carrier; the inheritance is read off the Go embedding so it cannot disagree with
// the struct.
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
