package identity

import (
	"testing"
)

func TestExecutorIdentitiesAreDistinctExactValues(t *testing.T) {
	const text = "process:root_1"
	member, err := ParseMember(text)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := ParseEffect(text)
	if err != nil {
		t.Fatal(err)
	}
	if member.String() != text || effect.String() != text {
		t.Fatalf("exact identities changed: %q/%q", member.String(), effect.String())
	}
	// The kinds that no caller carries are rules rather than values, and the
	// rule is one: the same text is legal for every executor identity.
	for _, validate := range []func(string) error{
		ValidateExecutor, ValidateMember, ValidateRequest, ValidateEffect,
	} {
		if err := validate(text); err != nil {
			t.Fatalf("executor identity rule rejected %q: %v", text, err)
		}
	}
}
