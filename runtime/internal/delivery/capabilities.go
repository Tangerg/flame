package delivery

import (
	"context"
	"reflect"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (e *Endpoint) enforceCapabilities(ctx context.Context, meta MethodMeta, parameters any) *Failure {
	for _, rule := range meta.CapabilityRules {
		if len(rule.When) != 0 && !matchesAll(rule.When, reflect.ValueOf(parameters)) {
			continue
		}
		client, _ := ClientCapabilitiesFrom(ctx)
		missing := protocol.MissingFeatureRequirements(e.handler.capabilities().Features, client, rule.Requires...)
		if len(missing) != 0 {
			return ProjectError(NewCapabilityGapError(missing...))
		}
	}
	return nil
}

func matchesAll(conditions []FieldCondition, parameters reflect.Value) bool {
	for _, condition := range conditions {
		if !condition.matches(parameters) {
			return false
		}
	}
	return true
}

func (f FieldCondition) matches(parameters reflect.Value) bool {
	value, found := lookupValue(parameters, f.Field)
	switch f.Operator {
	case OperatorPresent:
		return found && !isEmptyValue(value)
	case OperatorEquals:
		return found && value.Kind() == reflect.String && value.String() == f.Value
	default:
		return false
	}
}

func lookupValue(value reflect.Value, path string) (reflect.Value, bool) {
	current := value
	for segment := range strings.SplitSeq(path, ".") {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			if current.IsNil() {
				return reflect.Value{}, false
			}
			current = current.Elem()
		}
		if !current.IsValid() || current.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		field, ok := contractshape.LookupField(current.Type(), segment)
		if !ok {
			return reflect.Value{}, false
		}
		current = current.FieldByName(field.GoName)
	}
	return current, current.IsValid()
}

func isEmptyValue(value reflect.Value) bool {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	default:
		return value.IsZero()
	}
}
