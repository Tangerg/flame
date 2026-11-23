package dispatch

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type ConstraintKind string

const IdentityPattern = `^[^\p{C}\p{Z}]*$`

const (
	ConstraintNonEmpty              ConstraintKind = "nonEmpty"
	ConstraintPositive              ConstraintKind = "positive"
	ConstraintNonNegative           ConstraintKind = "nonNegative"
	ConstraintNonEmptyItems         ConstraintKind = "nonEmptyItems"
	ConstraintNonEmptyProperties    ConstraintKind = "nonEmptyProperties"
	ConstraintUniqueItems           ConstraintKind = "uniqueItems"
	ConstraintMinItems              ConstraintKind = "minItems"
	ConstraintMaxItems              ConstraintKind = "maxItems"
	ConstraintMaxLength             ConstraintKind = "maxLength"
	ConstraintMaxItemLength         ConstraintKind = "maxItemLength"
	ConstraintIdentity              ConstraintKind = "identity"
	ConstraintIdentityItems         ConstraintKind = "identityItems"
	ConstraintMaxPropertyNameLength ConstraintKind = "maxPropertyNameLength"
	ConstraintIdentityPropertyNames ConstraintKind = "identityPropertyNames"
	ConstraintPrefix                ConstraintKind = "prefix"
	ConstraintPatternItems          ConstraintKind = "patternItems"
	ConstraintPattern               ConstraintKind = "pattern"
	ConstraintMinimum               ConstraintKind = "minimum"
	ConstraintMaximum               ConstraintKind = "maximum"
)

func (c ConstraintKind) Valid() bool {
	_, _, ok := c.rule()
	return ok
}

type constraintArgument uint8

const (
	noArgument constraintArgument = iota
	limitArgument
	prefixArgument
	patternArgument
)

type constraintCarrier uint8

const (
	stringCarrier constraintCarrier = iota
	stringItemsCarrier
	stringKeysCarrier
	arrayCarrier
	objectCarrier
	numberCarrier
	signedNumberCarrier
)

func (c ConstraintKind) rule() (constraintArgument, constraintCarrier, bool) {
	rule, known := constraintRules[c]
	return rule.argument, rule.carrier, known
}

var constraintRules = map[ConstraintKind]struct {
	argument constraintArgument
	carrier  constraintCarrier
}{
	ConstraintNonEmpty:              {noArgument, stringCarrier},
	ConstraintIdentity:              {noArgument, stringCarrier},
	ConstraintMaxLength:             {limitArgument, stringCarrier},
	ConstraintPrefix:                {prefixArgument, stringCarrier},
	ConstraintPattern:               {patternArgument, stringCarrier},
	ConstraintIdentityItems:         {noArgument, stringItemsCarrier},
	ConstraintMaxItemLength:         {limitArgument, stringItemsCarrier},
	ConstraintPatternItems:          {patternArgument, stringItemsCarrier},
	ConstraintIdentityPropertyNames: {noArgument, stringKeysCarrier},
	ConstraintMaxPropertyNameLength: {limitArgument, stringKeysCarrier},
	ConstraintNonEmptyItems:         {noArgument, arrayCarrier},
	ConstraintUniqueItems:           {noArgument, arrayCarrier},
	ConstraintMinItems:              {limitArgument, arrayCarrier},
	ConstraintMaxItems:              {limitArgument, arrayCarrier},
	ConstraintNonEmptyProperties:    {noArgument, objectCarrier},
	ConstraintPositive:              {noArgument, numberCarrier},
	ConstraintMaximum:               {limitArgument, numberCarrier},
	ConstraintNonNegative:           {noArgument, signedNumberCarrier},
	ConstraintMinimum:               {limitArgument, signedNumberCarrier},
}

func (c constraintCarrier) carries(value reflect.Type) bool {
	switch c {
	case stringCarrier:
		return value.Kind() == reflect.String
	case stringItemsCarrier:
		return value.Kind() == reflect.Slice && value.Elem().Kind() == reflect.String
	case stringKeysCarrier:
		return value.Kind() == reflect.Map && value.Key().Kind() == reflect.String
	case arrayCarrier:
		return value.Kind() == reflect.Slice
	case objectCarrier:
		return value.Kind() == reflect.Map
	case numberCarrier:
		return slices.Contains([]reflect.Kind{reflect.Int, reflect.Int64, reflect.Uint64, reflect.Float64}, value.Kind())
	case signedNumberCarrier:
		return slices.Contains([]reflect.Kind{reflect.Int, reflect.Int64, reflect.Float64}, value.Kind())
	default:
		return false
	}
}

func (c constraintCarrier) String() string {
	switch c {
	case stringCarrier:
		return "a string"
	case stringItemsCarrier:
		return "a string array"
	case stringKeysCarrier:
		return "a string-keyed map"
	case arrayCarrier:
		return "an array"
	case objectCarrier:
		return "an object map"
	case numberCarrier:
		return "a number"
	case signedNumberCarrier:
		return "a signed number"
	default:
		return fmt.Sprintf("constraintCarrier(%d)", uint8(c))
	}
}

func (c ConstraintKind) String() string {
	if !c.Valid() {
		return fmt.Sprintf("ConstraintKind(%q)", string(c))
	}
	return string(c)
}

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
	case ConstraintPrefix, ConstraintPattern, ConstraintPatternItems:
		return fmt.Sprintf("%s(%s)", f.Kind, strconv.Quote(f.Value))
	default:
		return f.Kind.String()
	}
}

// Closed-enum membership is not declared here: it is derived from the enum's value
// set, and declaring it twice would let the two disagree.
type FieldConstraintSpec struct {
	GoType      reflect.Type
	Constraints []FieldConstraint
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
	fields, ok := protocol.WirePathFields(shape, constraint.Field)
	if !ok {
		return fmt.Errorf("%s: no JSON field %q", owner, constraint.Field)
	}
	if err := validateConstraintPathProjection(owner, constraint.Field, fields); err != nil {
		return err
	}
	argument, carrier, ok := constraint.Kind.rule()
	if !ok {
		return fmt.Errorf("%s.%s has invalid constraint kind %s", owner, constraint.Field, constraint.Kind)
	}
	if err := validateConstraintArguments(owner, constraint, argument); err != nil {
		return err
	}
	field := fields[len(fields)-1]
	valueType := field.Type
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if !carrier.carries(valueType) {
		return fmt.Errorf("%s.%s is %s; constraint %s requires %s", owner, constraint.Field, valueType, constraint.Kind, carrier)
	}
	return validateConstraintProjection(owner, field, constraint)
}

func validateConstraintPathProjection(owner, path string, fields []protocol.WireField) error {
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

func validateConstraintArguments(owner string, constraint FieldConstraint, argument constraintArgument) error {
	takesLimit := argument == limitArgument
	if takesLimit && constraint.Limit <= 0 {
		return fmt.Errorf("%s.%s constraint %s needs a positive limit", owner, constraint.Field, constraint.Kind)
	}
	if !takesLimit && constraint.Limit != 0 {
		return fmt.Errorf("%s.%s constraint %s does not accept a limit", owner, constraint.Field, constraint.Kind)
	}
	takesValue := argument == prefixArgument || argument == patternArgument
	if takesValue && constraint.Value == "" {
		return fmt.Errorf("%s.%s constraint %s needs a non-empty value", owner, constraint.Field, constraint.Kind)
	}
	if !takesValue && constraint.Value != "" {
		return fmt.Errorf("%s.%s constraint %s does not accept a value", owner, constraint.Field, constraint.Kind)
	}
	if argument == patternArgument {
		if _, err := regexp.Compile(constraint.Value); err != nil {
			return fmt.Errorf("%s.%s constraint %s has invalid pattern: %w", owner, constraint.Field, constraint.Kind, err)
		}
	}
	return nil
}

func validateConstraintProjection(owner string, field protocol.WireField, constraint FieldConstraint) error {
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
	if constraint.Kind == ConstraintMinimum && field.Optional {
		return fmt.Errorf(
			"%s.%s constraint %s does not support an optional value field",
			owner, constraint.Field, constraint.Kind,
		)
	}
	return validateTextualConstraintProjection(owner, field, constraint)
}

func validateConstraintHelperAssignment(owner string, field protocol.WireField, constraint FieldConstraint) error {
	if field.Type.Kind() == reflect.Pointer {
		target := field.Type.Elem()
		if target.Kind() == reflect.String && target != reflect.TypeFor[string]() {
			switch constraint.Kind {
			case ConstraintMaxLength, ConstraintIdentity, ConstraintPrefix, ConstraintPattern:
				return fmt.Errorf(
					"%s.%s constraint %s does not support a named string pointer",
					owner, constraint.Field, constraint.Kind,
				)
			case ConstraintNonEmpty, ConstraintPositive, ConstraintNonNegative, ConstraintNonEmptyItems, ConstraintNonEmptyProperties, ConstraintUniqueItems, ConstraintMinItems, ConstraintMaxItems, ConstraintMaxItemLength, ConstraintIdentityItems, ConstraintMaxPropertyNameLength, ConstraintIdentityPropertyNames, ConstraintPatternItems, ConstraintMinimum, ConstraintMaximum:
			}
		}
		if target.Kind() == reflect.Slice && target.Name() != "" {
			switch constraint.Kind {
			case ConstraintUniqueItems, ConstraintMaxItems, ConstraintMaxItemLength, ConstraintPatternItems:
				return fmt.Errorf(
					"%s.%s constraint %s does not support a named slice pointer",
					owner, constraint.Field, constraint.Kind,
				)
			case ConstraintNonEmpty, ConstraintPositive, ConstraintNonNegative, ConstraintNonEmptyItems, ConstraintNonEmptyProperties, ConstraintMinItems, ConstraintMaxLength, ConstraintIdentity, ConstraintIdentityItems, ConstraintMaxPropertyNameLength, ConstraintIdentityPropertyNames, ConstraintPrefix, ConstraintPattern, ConstraintMinimum, ConstraintMaximum:
			}
		}
	}
	if _, carrier, _ := constraint.Kind.rule(); carrier == objectCarrier || carrier == stringKeysCarrier {
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

func validatePointerConstraintProjection(owner string, field protocol.WireField, constraint FieldConstraint) error {
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
		ConstraintMaxPropertyNameLength, ConstraintIdentityPropertyNames, ConstraintMinimum,
		ConstraintUniqueItems, ConstraintMaxItems, ConstraintPatternItems:
		return fmt.Errorf(
			"%s.%s constraint %s does not support a pointer target",
			owner, constraint.Field, constraint.Kind,
		)
	default:
		return nil
	}
}

func validateTextualConstraintProjection(owner string, field protocol.WireField, constraint FieldConstraint) error {
	declaredKind := field.Type.Kind()
	if constraint.Kind == ConstraintPrefix && declaredKind != reflect.Pointer && field.Optional {
		return fmt.Errorf(
			"%s.%s constraint %s does not support an optional value field",
			owner, constraint.Field, constraint.Kind,
		)
	}
	if constraint.Kind == ConstraintPattern && declaredKind == reflect.Pointer {
		return fmt.Errorf(
			"%s.%s constraint %s does not support a pointer string",
			owner, constraint.Field, constraint.Kind,
		)
	}
	if constraint.Kind == ConstraintIdentityItems && declaredKind == reflect.Pointer {
		return fmt.Errorf(
			"%s.%s constraint %s does not support a pointer string array",
			owner, constraint.Field, constraint.Kind,
		)
	}
	return nil
}
