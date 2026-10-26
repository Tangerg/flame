package delivery

import "testing"

func TestRuntimeInstanceRoundTrip(t *testing.T) {
	first := NewInstanceID()
	second := NewInstanceID()
	if first == second {
		t.Fatalf("two Runtime instances received %q", first)
	}
	if err := ValidateInstanceID(first); err != nil {
		t.Fatalf("generated identity is invalid: %v", err)
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
			if err := ValidateInstanceID(test.text); err == nil {
				t.Fatalf("Parse(%q) succeeded", test.text)
			}
		})
	}
}
