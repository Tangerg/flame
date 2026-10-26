package runs

import (
	"strings"
	"testing"
)

func TestExecutorIdentityEnvelope(t *testing.T) {
	for _, executorIdentityValue := range []string{"model:root:19", "tool:root:1", "AZaz09._:-", strings.Repeat("x", MaximumIdentityBytes)} {
		if err := ValidateInvocation(executorIdentityValue); err != nil {
			t.Errorf("valid identity %q: %v", executorIdentityValue, err)
		}
	}
	for _, executorIdentityValue := range []string{"", "call~1", "call/1", "call%3A1", "call 1", "call\n", "call\x00", "调用", strings.Repeat("x", MaximumIdentityBytes+1)} {
		if err := ValidateInvocation(executorIdentityValue); err == nil {
			t.Errorf("invalid identity %q accepted", executorIdentityValue)
		}
	}
}

func TestExecutorIdentitiesAreDistinctExactValues(t *testing.T) {
	const text = "process:root_1"
	member, err := ParseMember(text)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := ParseInvocation(text)
	if err != nil {
		t.Fatal(err)
	}
	if member.String() != text || effect.String() != text {
		t.Fatalf("exact identities changed: %q/%q", member.String(), effect.String())
	}
	// The kinds that no caller carries are rules rather than values, and the
	// rule is one: the same text is legal for every executor identity.
	for _, validateExecutionIdentity := range []func(string) error{
		ValidateExecutor, ValidateMember, validateRequest, ValidateInvocation,
	} {
		if err := validateExecutionIdentity(text); err != nil {
			t.Fatalf("executor identity rule rejected %q: %v", text, err)
		}
	}
}
