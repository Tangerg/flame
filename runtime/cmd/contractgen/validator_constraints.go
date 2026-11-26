package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

func constraintCheck(shape reflect.Type, constraint dispatch.FieldConstraint) validatorCheck {
	selector, leaf, found := protocol.WireGoPath(shape, constraint.Field)
	if !found {
		panic(fmt.Sprintf("contractgen: %s has no field %q", shape.Name(), constraint.Field))
	}

	value := receiverName(shape) + "." + selector
	valueType := leaf.Type
	for valueType.Kind() == reflect.Pointer {
		value = "*" + value
		valueType = valueType.Elem()
	}
	if valueType.Kind() == reflect.String && valueType != reflect.TypeFor[string]() {
		value = fmt.Sprintf("string(%s)", value)
	}

	arguments := []string{strconv.Quote(constraint.Field), value}
	if constraint.Value != "" {
		arguments = append(arguments, strconv.Quote(constraint.Value))
	}
	if constraint.Limit != 0 {
		arguments = append(arguments, fmt.Sprint(constraint.Limit))
	}
	helper := constraintHelper(constraint.Kind)
	check := validatorCheck{expression: fmt.Sprintf("%s(%s)", helper, strings.Join(arguments, ", "))}
	if leaf.Optional || strings.Contains(constraint.Field, ".") {
		check.present = constraint.Field
	}
	return check
}

func constraintHelper(kind dispatch.ConstraintKind) string {
	switch kind {
	case dispatch.ConstraintNonEmpty:
		return "nonEmptyText"
	case dispatch.ConstraintMaxLength:
		return "maxLength"
	case dispatch.ConstraintIdentity:
		return "identity"
	case dispatch.ConstraintPrefix:
		return "textPrefix"
	case dispatch.ConstraintPattern:
		return "textPattern"
	case dispatch.ConstraintPositive:
		return "positiveNumber"
	case dispatch.ConstraintNonNegative:
		return "nonNegativeNumber"
	case dispatch.ConstraintMinimum:
		return "minimumNumber"
	case dispatch.ConstraintMaximum:
		return "maximumNumber"
	case dispatch.ConstraintNonEmptyItems:
		return "nonEmptyItems"
	case dispatch.ConstraintUniqueItems:
		return "uniqueItems"
	case dispatch.ConstraintMinItems:
		return "minItems"
	case dispatch.ConstraintMaxItems:
		return "maxItems"
	case dispatch.ConstraintMaxItemLength:
		return "maxItemLength"
	case dispatch.ConstraintIdentityItems:
		return "identityItems"
	case dispatch.ConstraintPatternItems:
		return "textPatternItems"
	case dispatch.ConstraintNonEmptyProperties:
		return "nonEmptyProperties"
	case dispatch.ConstraintMaxPropertyNameLength:
		return "maxPropertyNameLength"
	case dispatch.ConstraintIdentityPropertyNames:
		return "identityPropertyNames"
	default:
		panic(fmt.Sprintf("contractgen: unsupported Go validator for %s", kind))
	}
}
