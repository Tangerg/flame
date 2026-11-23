package dispatch

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/runtime/internal/delivery"
)

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
	if err := validateFieldRoles(owner, rule); err != nil {
		return err
	}
	for _, field := range slices.Concat(rule.Required, rule.RequiredAny, rule.Forbidden) {
		if err := protocol.ValidateWirePath(shape, field); err != nil {
			return fmt.Errorf("%s: %w", owner, err)
		}
	}
	return validateAllowedValueSets(owner, shape, rule.AllowedValues, rule.Forbidden)
}

func validateFieldRoles(owner string, rule ConditionalRule) error {
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
	return nil
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
		if err := validateAllowedValueSet(owner, shape, set, sets[:index], forbidden); err != nil {
			return err
		}
	}
	return nil
}

func validateAllowedValueSet(
	owner string,
	shape reflect.Type,
	set AllowedValueSet,
	previous []AllowedValueSet,
	forbidden []string,
) error {
	if slices.ContainsFunc(previous, func(previous AllowedValueSet) bool {
		return previous.Field == set.Field
	}) {
		return fmt.Errorf("%s: allowed values for field %q are declared twice", owner, set.Field)
	}
	if forbiddenField, found := overlappingForbiddenField(set.Field, forbidden); found {
		return fmt.Errorf(
			"%s: allowed-values field %q conflicts with forbidden field %q",
			owner, set.Field, forbiddenField,
		)
	}
	_, leaf, found := protocol.WireGoPath(shape, set.Field)
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
	return validateAllowedValues(owner, set)
}

func overlappingForbiddenField(field string, forbidden []string) (string, bool) {
	for _, forbiddenField := range forbidden {
		if fieldPathAtOrBelow(field, forbiddenField) || fieldPathAtOrBelow(forbiddenField, field) {
			return forbiddenField, true
		}
	}
	return "", false
}

func validateAllowedValues(owner string, set AllowedValueSet) error {
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
	return nil
}

func validateVariantAllowedValueSets(owner string, shape reflect.Type, variant VariantSpec) error {
	if err := validateAllowedValueSets(owner, shape, variant.AllowedValues, nil); err != nil {
		return err
	}
	claimed := slices.Concat(variant.Required, variant.Optional)
	for _, set := range variant.AllowedValues {
		if !slices.ContainsFunc(claimed, func(path string) bool {
			return fieldPathAtOrBelow(set.Field, path)
		}) {
			return fmt.Errorf(
				"%s: allowed-values field %q is not claimed by the variant",
				owner, set.Field,
			)
		}
	}
	return nil
}

func fieldPathAtOrBelow(path, ancestor string) bool {
	return path == ancestor || strings.HasPrefix(path, ancestor+".")
}

func whenEquals(field, value string) []delivery.FieldCondition {
	return []delivery.FieldCondition{{Field: field, Operator: delivery.OperatorEquals, Value: value}}
}

func whenPresent(field string) []delivery.FieldCondition {
	return []delivery.FieldCondition{{Field: field, Operator: delivery.OperatorPresent}}
}
