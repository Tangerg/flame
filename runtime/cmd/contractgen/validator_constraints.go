package main

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

func constraintCheck(
	shape reflect.Type,
	constraint dispatch.FieldConstraint,
	requiredByShape bool,
) string {
	selector, leaf, found := protocol.WireGoPath(shape, constraint.Field)
	if !found {
		panic(fmt.Sprintf("contractgen: %s has no field %q", shape.Name(), constraint.Field))
	}
	switch constraint.Kind {
	case dispatch.ConstraintNonEmpty, dispatch.ConstraintMaxLength, dispatch.ConstraintIdentity,
		dispatch.ConstraintPrefix, dispatch.ConstraintPattern:
		return textConstraintCheck(shape, selector, leaf, constraint, requiredByShape)
	case dispatch.ConstraintPositive, dispatch.ConstraintNonNegative,
		dispatch.ConstraintMinimum, dispatch.ConstraintMaximum:
		return numericConstraintCheck(shape, selector, leaf, constraint)
	case dispatch.ConstraintNonEmptyItems, dispatch.ConstraintUniqueItems,
		dispatch.ConstraintMinItems, dispatch.ConstraintMaxItems,
		dispatch.ConstraintMaxItemLength, dispatch.ConstraintIdentityItems,
		dispatch.ConstraintPatternItems:
		return itemConstraintCheck(shape, selector, leaf, constraint)
	case dispatch.ConstraintNonEmptyProperties, dispatch.ConstraintMaxPropertyNameLength,
		dispatch.ConstraintIdentityPropertyNames:
		return propertyConstraintCheck(shape, selector, constraint)
	default:
		return unsupportedConstraintCheck(shape, constraint)
	}
}

func textConstraintCheck(
	shape reflect.Type,
	selector string,
	leaf protocol.WireField,
	constraint dispatch.FieldConstraint,
	requiredByShape bool,
) string {
	field := strconv.Quote(constraint.Field)
	switch constraint.Kind {
	case dispatch.ConstraintNonEmpty:
		return nonEmptyConstraintCheck(shape, selector, leaf, constraint, requiredByShape)
	case dispatch.ConstraintMaxLength:
		validatorName := "maxLength"
		if leaf.Type.Kind() == reflect.Pointer {
			validatorName = "optionalMaxLength"
		}
		return fmt.Sprintf(
			"%s(%s, %s, %d)",
			validatorName,
			field,
			stringExpr(shape, selector, leaf.Type),
			constraint.Limit,
		)
	case dispatch.ConstraintIdentity:
		validatorName := "identity"
		if leaf.Type.Kind() == reflect.Pointer {
			validatorName = "optionalIdentity"
		}
		return fmt.Sprintf("%s(%s, %s)", validatorName, field, stringExpr(shape, selector, leaf.Type))
	case dispatch.ConstraintPrefix:
		return prefixConstraintCheck(shape, selector, leaf, constraint)
	case dispatch.ConstraintPattern:
		return patternConstraintCheck(shape, selector, leaf, constraint)
	default:
		return unsupportedConstraintCheck(shape, constraint)
	}
}

func nonEmptyConstraintCheck(
	shape reflect.Type,
	selector string,
	leaf protocol.WireField,
	constraint dispatch.FieldConstraint,
	requiredByShape bool,
) string {
	// A non-pointer scalar cannot distinguish omission from zero after decoding.
	// requiredWhen owns its branch-specific requiredness; an unconditional
	// requiredText would reject legal branches.
	if leaf.Optional && leaf.Type.Kind() != reflect.Pointer && requiredByShape {
		return ""
	}
	validatorName := "requiredText"
	if leaf.Optional && leaf.Type.Kind() == reflect.Pointer {
		validatorName = "optionalText"
	}
	return fmt.Sprintf(
		"%s(%s, %s)",
		validatorName,
		strconv.Quote(constraint.Field),
		stringExpr(shape, selector, leaf.Type),
	)
}

func prefixConstraintCheck(
	shape reflect.Type,
	selector string,
	leaf protocol.WireField,
	constraint dispatch.FieldConstraint,
) string {
	validatorName := "requiredTextPrefix"
	if leaf.Type.Kind() == reflect.Pointer {
		validatorName = "optionalTextPointerPrefix"
	}
	return fmt.Sprintf(
		"%s(%s, %s, %s)",
		validatorName,
		strconv.Quote(constraint.Field),
		stringExpr(shape, selector, leaf.Type),
		strconv.Quote(constraint.Value),
	)
}

func patternConstraintCheck(
	shape reflect.Type,
	selector string,
	leaf protocol.WireField,
	constraint dispatch.FieldConstraint,
) string {
	validatorName := "requiredTextPattern"
	if leaf.Optional {
		validatorName = "optionalTextPattern"
	}
	return fmt.Sprintf(
		"%s(%s, %s, %s)",
		validatorName,
		strconv.Quote(constraint.Field),
		stringExpr(shape, selector, leaf.Type),
		strconv.Quote(constraint.Value),
	)
}

func numericConstraintCheck(
	shape reflect.Type,
	selector string,
	leaf protocol.WireField,
	constraint dispatch.FieldConstraint,
) string {
	field := strconv.Quote(constraint.Field)
	ref := receiverName(shape) + "." + selector
	switch constraint.Kind {
	case dispatch.ConstraintPositive:
		validatorName := "positiveNumber"
		if leaf.Type.Kind() == reflect.Pointer {
			validatorName = "optionalPositiveNumber"
		} else if leaf.Optional {
			validatorName = "optionalPositiveScalarNumber"
		}
		return fmt.Sprintf("%s(%s, %s)", validatorName, field, ref)
	case dispatch.ConstraintNonNegative:
		validatorName := "nonNegativeNumber"
		if leaf.Type.Kind() == reflect.Pointer {
			validatorName = "optionalNonNegativeNumber"
		}
		return fmt.Sprintf("%s(%s, %s)", validatorName, field, ref)
	case dispatch.ConstraintMinimum:
		return fmt.Sprintf("minimumNumber(%s, %s, %d)", field, ref, constraint.Limit)
	case dispatch.ConstraintMaximum:
		validatorName := "maximumNumber"
		if leaf.Type.Kind() == reflect.Pointer {
			validatorName = "optionalMaximumNumber"
		}
		return fmt.Sprintf("%s(%s, %s, %d)", validatorName, field, ref, constraint.Limit)
	default:
		return unsupportedConstraintCheck(shape, constraint)
	}
}

func itemConstraintCheck(
	shape reflect.Type,
	selector string,
	leaf protocol.WireField,
	constraint dispatch.FieldConstraint,
) string {
	field := strconv.Quote(constraint.Field)
	ref := receiverName(shape) + "." + selector
	switch constraint.Kind {
	case dispatch.ConstraintNonEmptyItems:
		validatorName := "requiredItems"
		if leaf.Optional {
			validatorName = "nonEmptyItems"
		}
		return fmt.Sprintf("%s(%s, %s)", validatorName, field, ref)
	case dispatch.ConstraintUniqueItems:
		return fmt.Sprintf("uniqueItems(%s, %s)", field, ref)
	case dispatch.ConstraintMinItems:
		return fmt.Sprintf("optionalMinItems(%s, %s, %d)", field, ref, constraint.Limit)
	case dispatch.ConstraintMaxItems:
		return fmt.Sprintf("maxItems(%s, %s, %d)", field, ref, constraint.Limit)
	case dispatch.ConstraintMaxItemLength:
		validatorName := "maxItemLength"
		if leaf.Type.Kind() == reflect.Pointer {
			validatorName = "optionalMaxItemLength"
		}
		return fmt.Sprintf("%s(%s, %s, %d)", validatorName, field, ref, constraint.Limit)
	case dispatch.ConstraintIdentityItems:
		return fmt.Sprintf("identityItems(%s, %s)", field, ref)
	case dispatch.ConstraintPatternItems:
		return fmt.Sprintf(
			"textPatternItems(%s, %s, %s)",
			field,
			ref,
			strconv.Quote(constraint.Value),
		)
	default:
		return unsupportedConstraintCheck(shape, constraint)
	}
}

func propertyConstraintCheck(
	shape reflect.Type,
	selector string,
	constraint dispatch.FieldConstraint,
) string {
	field := strconv.Quote(constraint.Field)
	ref := receiverName(shape) + "." + selector
	switch constraint.Kind {
	case dispatch.ConstraintNonEmptyProperties:
		return fmt.Sprintf("nonEmptyProperties(%s, %s)", field, ref)
	case dispatch.ConstraintMaxPropertyNameLength:
		return fmt.Sprintf("maxPropertyNameLength(%s, %s, %d)", field, ref, constraint.Limit)
	case dispatch.ConstraintIdentityPropertyNames:
		return fmt.Sprintf("identityPropertyNames(%s, %s)", field, ref)
	default:
		return unsupportedConstraintCheck(shape, constraint)
	}
}

func unsupportedConstraintCheck(shape reflect.Type, constraint dispatch.FieldConstraint) string {
	panic(fmt.Sprintf(
		"contractgen: %s.%s uses unsupported constraint %s",
		shape.Name(),
		constraint.Field,
		constraint.Kind,
	))
}
