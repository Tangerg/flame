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
	// A single component is legal for every identity; only members carry paths.
	for _, validateExecutionIdentity := range []func(string) error{
		ValidateExecutor, ValidateMember, validateRequest, ValidateInvocation,
	} {
		if err := validateExecutionIdentity(text); err != nil {
			t.Fatalf("executor identity rule rejected %q: %v", text, err)
		}
	}
}

func TestExecutorMemberIdentityPreservesNestedPaths(t *testing.T) {
	component := strings.Repeat("x", MaximumIdentityBytes)
	for _, path := range []string{"root/child", "root/delegate/interaction/child", component + "/" + component} {
		member, err := ParseMember(path)
		if err != nil {
			t.Fatalf("parse member %q: %v", path, err)
		}
		if member.String() != path || member.Validate() != nil {
			t.Fatalf("member path changed: %q", member.String())
		}
		if err := ValidateInvocation(path); err == nil {
			t.Fatalf("invocation accepted member path %q", path)
		}
	}
	for _, path := range []string{"", "/root", "root/", "root//child", "root/child name", "root/" + component + "x"} {
		if _, err := ParseMember(path); err == nil {
			t.Fatalf("invalid member path %q accepted", path)
		}
	}
}
