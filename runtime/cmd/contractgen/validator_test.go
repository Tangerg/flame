package main

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

type validatorUnionFixture struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Alpha string `json:"alpha,omitempty"`
	Beta  string `json:"beta,omitempty"`
}

type validatorEmbeddedFixture struct {
	Cursor string `json:"cursor,omitempty"`
}

type validatorOuterFixture struct {
	validatorEmbeddedFixture
	Search string `json:"search,omitempty"`
}

func TestValueConstraintsFollowFlattenedEmbedding(t *testing.T) {
	t.Parallel()

	embedded := reflect.TypeFor[validatorEmbeddedFixture]()
	outer := reflect.TypeFor[validatorOuterFixture]()
	constraints := inheritedValueConstraints(outer, map[reflect.Type][]dispatch.FieldConstraint{
		embedded: {{Field: "cursor", Kind: dispatch.ConstraintMaxLength, Limit: 64}},
		outer:    {{Field: "search", Kind: dispatch.ConstraintNonEmpty}},
	})
	want := []dispatch.FieldConstraint{
		{Field: "search", Kind: dispatch.ConstraintNonEmpty},
		{Field: "cursor", Kind: dispatch.ConstraintMaxLength, Limit: 64},
	}
	if !slices.Equal(constraints, want) {
		t.Fatalf("inherited constraints = %v, want %v", constraints, want)
	}
}

func TestUnionBranchPresenceDoesNotBecomeGlobalRequiredness(t *testing.T) {
	t.Parallel()

	shape := reflect.TypeFor[validatorUnionFixture]()
	checks := validatorChecks(
		shape,
		[]dispatch.FieldConstraint{
			{Field: "id", Kind: dispatch.ConstraintNonEmpty},
			{Field: "alpha", Kind: dispatch.ConstraintNonEmpty},
			{Field: "beta", Kind: dispatch.ConstraintNonEmpty},
		},
		dispatch.UnionSpec{
			GoType: shape, Discriminator: "type",
			Variants: []dispatch.VariantSpec{
				{Tag: "alpha", Required: []string{"id", "alpha"}},
				{Tag: "beta", Required: []string{"id", "beta"}},
			},
		},
		nil,
	)

	if !slices.Contains(checks, `requiredText("id", v.ID)`) {
		t.Fatalf("non-optional identity lost its value constraint: %v", checks)
	}
	for _, forbidden := range []string{
		`requiredText("alpha", v.Alpha)`,
		`requiredText("beta", v.Beta)`,
	} {
		if slices.Contains(checks, forbidden) {
			t.Fatalf("branch field became globally required through %s", forbidden)
		}
	}
	for _, required := range []string{
		`requiredWhen(wireFieldEquals(v, "type", "alpha"), "alpha", v)`,
		`requiredWhen(wireFieldEquals(v, "type", "beta"), "beta", v)`,
	} {
		if !slices.Contains(checks, required) {
			t.Fatalf("branch requiredness was not generated through %s: %v", required, checks)
		}
	}
}

func TestObjectAlternativePresenceStaysOneRule(t *testing.T) {
	t.Parallel()

	checks := objectChecks([]dispatch.ConditionalRule{{
		RequiredAny: []string{"alpha", "beta"},
	}}, "v")
	want := `requiredAnyWhen(true, []string{"alpha", "beta"}, v)`
	if !slices.Equal(checks, []string{want}) {
		t.Fatalf("object checks = %v, want one alternative-presence check %q", checks, want)
	}
}
