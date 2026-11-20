package dispatch

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

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
	_, _, ok := c.rule()
	return ok
}

// constraintArgument is the single parameter a constraint kind takes.
type constraintArgument uint8

const (
	noArgument constraintArgument = iota
	limitArgument
	prefixArgument
	patternArgument
)

// constraintCarrier is the wire shape a constraint kind constrains.
type constraintCarrier uint8

const (
	stringCarrier constraintCarrier = iota
	stringItemsCarrier
	stringKeysCarrier
	arrayCarrier
	objectCarrier
	numberCarrier
	// signedNumberCarrier excludes uint64, for which a lower bound is vacuous.
	signedNumberCarrier
)

// rule is what a kind demands of its declaration; every declaration check
// derives from it, so a new kind is described in exactly one place.
func (c ConstraintKind) rule() (constraintArgument, constraintCarrier, bool) {
	rule, known := constraintRules[c]
	return rule.argument, rule.carrier, known
}

// constraintRules is the one statement of what each constraint kind takes and
// which value shape carries it; validity and every declaration check derive
// from it.
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
	ConstraintPrefixItems:           {prefixArgument, stringItemsCarrier},
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
	if (constraint.Kind == ConstraintIdentityItems || constraint.Kind == ConstraintPrefixItems) &&
		declaredKind == reflect.Pointer {
		return fmt.Errorf(
			"%s.%s constraint %s does not support a pointer string array",
			owner, constraint.Field, constraint.Kind,
		)
	}
	return nil
}
