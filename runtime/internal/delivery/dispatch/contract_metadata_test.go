package dispatch

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

type allowedValuesListFixture struct {
	Values []string `json:"values,omitempty"`
}

type constraintProjectionFixture struct {
	RequiredPointer *string   `json:"requiredPointer"`
	OptionalPointer *string   `json:"optionalPointer,omitempty"`
	OptionalValue   string    `json:"optionalValue,omitempty"`
	PointerItems    *[]string `json:"pointerItems,omitempty"`
}

func TestShapeMetadataRejectsUnsupportedValidatorTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		constraint FieldConstraint
		want       string
	}{
		{
			name: "required pointer prefix",
			constraint: FieldConstraint{
				Field: "requiredPointer", Kind: ConstraintPrefix, Value: "id_",
			},
			want: "required pointer",
		},
		{
			name: "required pointer pattern",
			constraint: FieldConstraint{
				Field: "requiredPointer", Kind: ConstraintPattern, Value: `\S`,
			},
			want: "required pointer",
		},
		{
			name: "optional value prefix",
			constraint: FieldConstraint{
				Field: "optionalValue", Kind: ConstraintPrefix, Value: "id_",
			},
			want: "optional value",
		},
		{
			name: "pointer identity items",
			constraint: FieldConstraint{
				Field: "pointerItems", Kind: ConstraintIdentityItems,
			},
			want: "pointer string array",
		},
		{
			name: "pointer prefix items",
			constraint: FieldConstraint{
				Field: "pointerItems", Kind: ConstraintPrefixItems, Value: "id_",
			},
			want: "pointer string array",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := (FieldConstraintSpec{
				GoType:      reflect.TypeFor[constraintProjectionFixture](),
				Constraints: []FieldConstraint{test.constraint},
			}).validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestShapeMetadataKeepsSupportedValidatorTargets(t *testing.T) {
	t.Parallel()

	for _, constraint := range []FieldConstraint{
		{Field: "optionalPointer", Kind: ConstraintPrefix, Value: "id_"},
		{Field: "optionalPointer", Kind: ConstraintPattern, Value: `\S`},
		{Field: "pointerItems", Kind: ConstraintPatternItems, Value: `\S`},
	} {
		err := (FieldConstraintSpec{
			GoType:      reflect.TypeFor[constraintProjectionFixture](),
			Constraints: []FieldConstraint{constraint},
		}).validate()
		if err != nil {
			t.Fatalf("validate %+v: %v", constraint, err)
		}
	}
}

func TestObjectConstraintRejectsImpossibleConditionSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		when []delivery.FieldCondition
		want string
	}{
		{
			name: "equals non-string",
			when: []delivery.FieldCondition{{
				Field: "retryAfterSeconds", Operator: delivery.OperatorEquals, Value: "1",
			}},
			want: "requires a string field",
		},
		{
			name: "conflicting equals",
			when: []delivery.FieldCondition{
				{Field: "type", Operator: delivery.OperatorEquals, Value: "one"},
				{Field: "type", Operator: delivery.OperatorEquals, Value: "two"},
			},
			want: "conflicting equals conditions",
		},
		{
			name: "redundant present and equals",
			when: []delivery.FieldCondition{
				{Field: "type", Operator: delivery.OperatorPresent},
				{Field: "type", Operator: delivery.OperatorEquals, Value: "one"},
			},
			want: "combines present and equals",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := (ObjectConstraintSpec{
				GoType: reflect.TypeFor[protocol.ProblemData](),
				Rules: []ConditionalRule{{
					When: test.when, Required: []string{"detail"},
				}},
			}).validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestShapeMetadataRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	valueSpec := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintKind("invalid"),
		}},
	}
	err := valueSpec.validate()
	if err == nil || !strings.Contains(err.Error(), `ConstraintKind("invalid")`) ||
		!strings.Contains(err.Error(), "GetRunRequest.runId") {
		t.Fatalf("value constraint error = %v, want shape, field and illegal kind", err)
	}
	if got := ConstraintKind("invalid").String(); got == ConstraintNonEmpty.String() {
		t.Fatalf("unknown constraint kind masquerades as %q", got)
	}

	bounded := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMinItems,
		}},
	}
	if validateErr := bounded.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "positive limit") {
		t.Fatalf("bounded constraint error = %v, want positive limit", validateErr)
	}

	unbounded := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintNonEmpty, Limit: 1,
		}},
	}

	missingPrefix := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintPrefix,
		}},
	}
	if validateErr := missingPrefix.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "non-empty value") {
		t.Fatalf("prefix constraint error = %v, want required value", validateErr)
	}

	invalidPattern := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintPattern, Value: "[",
		}},
	}
	if validateErr := invalidPattern.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "invalid pattern") {
		t.Fatalf("pattern constraint error = %v, want invalid pattern", validateErr)
	}

	unexpectedValue := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintNonEmpty, Value: "run_",
		}},
	}
	if validateErr := unexpectedValue.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "does not accept a value") {
		t.Fatalf("non-prefix constraint error = %v, want rejected value", validateErr)
	}
	if validateErr := unbounded.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "does not accept a limit") {
		t.Fatalf("unbounded constraint error = %v, want rejected limit", validateErr)
	}

	duplicateBound := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{
			{Field: "options", Kind: ConstraintMinItems, Limit: 2},
			{Field: "options", Kind: ConstraintMinItems, Limit: 3},
		},
	}
	if validateErr := duplicateBound.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "declares constraint minItems twice") {
		t.Fatalf("duplicate bounded constraint error = %v, want duplicate rejection", validateErr)
	}

	wrongType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMaxLength, Limit: 12,
		}},
	}
	if validateErr := wrongType.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "only a string has a length") {
		t.Fatalf("bounded constraint type error = %v, want string requirement", validateErr)
	}

	wrongMaximumType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMaximum, Limit: 3,
		}},
	}
	if validateErr := wrongMaximumType.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "only a number can have a maximum") {
		t.Fatalf("maximum constraint type error = %v, want numeric requirement", validateErr)
	}

	wrongMinimumType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMinimum, Limit: 3,
		}},
	}
	if validateErr := wrongMinimumType.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "only a number can have a minimum") {
		t.Fatalf("minimum constraint type error = %v, want numeric requirement", validateErr)
	}

	objectSpec := ObjectConstraintSpec{
		GoType: reflect.TypeFor[protocol.ProblemData](),
		Rules: []ConditionalRule{{
			When: []delivery.FieldCondition{{
				Field: "type", Operator: delivery.ConditionOperator("invalid"),
			}},
			Required: []string{"detail"},
		}},
	}
	err = objectSpec.validate()
	if err == nil || !strings.Contains(err.Error(), "ProblemData") ||
		!strings.Contains(err.Error(), "type") ||
		!strings.Contains(err.Error(), `ConditionOperator("invalid")`) {
		t.Fatalf("object constraint error = %v, want shape, field and illegal operator", err)
	}

	allowedSpec := ObjectConstraintSpec{
		GoType: reflect.TypeFor[protocol.ProblemData](),
		Rules: []ConditionalRule{{
			AllowedValues: []AllowedValueSet{{Field: "retryAfterSeconds", Values: []string{"1"}}},
		}},
	}
	if validateErr := allowedSpec.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "not a string") {
		t.Fatalf("allowed-values type error = %v, want string requirement", validateErr)
	}

	listAllowedSpec := ObjectConstraintSpec{
		GoType: reflect.TypeFor[allowedValuesListFixture](),
		Rules: []ConditionalRule{{
			AllowedValues: []AllowedValueSet{{Field: "values", Values: []string{"one"}}},
		}},
	}
	if validateErr := listAllowedSpec.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "not a string") {
		t.Fatalf("list allowed-values type error = %v, want scalar string requirement", validateErr)
	}

	for _, test := range []struct {
		name string
		rule ConditionalRule
		want string
	}{
		{
			name: "duplicate required-any",
			rule: ConditionalRule{RequiredAny: []string{"detail", "detail"}},
			want: "required-any field \"detail\" is declared twice",
		},
		{
			name: "required and required-any",
			rule: ConditionalRule{Required: []string{"detail"}, RequiredAny: []string{"detail"}},
			want: "both required and required-any",
		},
		{
			name: "required-any and forbidden",
			rule: ConditionalRule{RequiredAny: []string{"detail"}, Forbidden: []string{"detail"}},
			want: "both required-any and forbidden",
		},
		{
			name: "unknown required-any path",
			rule: ConditionalRule{RequiredAny: []string{"missing"}},
			want: "no JSON field \"missing\"",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := (ObjectConstraintSpec{
				GoType: reflect.TypeFor[protocol.ProblemData](),
				Rules:  []ConditionalRule{test.rule},
			}).validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want %q", err, test.want)
			}
		})
	}
}
