package identity

import "testing"

func TestRuntimeInstanceRoundTrip(t *testing.T) {
	first := NewRuntimeInstance()
	second := NewRuntimeInstance()
	if first.String() == second.String() {
		t.Fatalf("two Runtime instances received %q", first.String())
	}
	parsed, err := ParseRuntimeInstance(first.String())
	if err != nil || parsed != first {
		t.Fatalf("Parse(New()) = (%q, %v), want %q", parsed.String(), err, first.String())
	}
}

func TestParseRejectsNonCanonicalIdentityMaterial(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "empty"},
		{name: "old loose fixture", text: "runtime_test"},
		{name: "missing prefix", text: "00000000-0000-0000-0000-000000000000"},
		{name: "uppercase UUID", text: "runtime_00000000-0000-0000-0000-00000000000A"},
		{name: "leading whitespace", text: " runtime_00000000-0000-0000-0000-000000000000"},
		{name: "trailing whitespace", text: "runtime_00000000-0000-0000-0000-000000000000 "},
		{name: "non UUID", text: "runtime_00000000-0000-0000-0000-00000000000z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseRuntimeInstance(test.text); err == nil {
				t.Fatalf("Parse(%q) succeeded", test.text)
			}
		})
	}
}
