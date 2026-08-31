package executoridentity

import (
	"testing"
)

func TestExecutorIdentitiesAreDistinctExactValues(t *testing.T) {
	const text = "process:root_1"
	member, err := ParseMember(text)
	if err != nil {
		t.Fatal(err)
	}
	request, err := ParseRequest(text)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := ParseEffect(text)
	if err != nil {
		t.Fatal(err)
	}
	if member.String() != text || request.String() != text || effect.String() != text {
		t.Fatalf("exact identities changed: %q/%q/%q", member.String(), request.String(), effect.String())
	}
}
