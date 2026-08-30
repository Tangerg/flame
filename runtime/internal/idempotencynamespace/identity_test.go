package idempotencynamespace

import (
	"regexp"
	"testing"
)

const validNamespace = "idp_11111111111111111111111111111111"

func TestIdentityRoundTrip(t *testing.T) {
	if !regexp.MustCompile(Pattern).MatchString(validNamespace) {
		t.Fatalf("public pattern rejects canonical namespace %q", validNamespace)
	}
	parsed, err := Parse(validNamespace)
	if err != nil || parsed.String() != validNamespace {
		t.Fatalf("Parse(%q) = (%q, %v)", validNamespace, parsed.String(), err)
	}
}

func TestParseRejectsNonCanonicalMaterial(t *testing.T) {
	pattern := regexp.MustCompile(Pattern)
	tests := []struct {
		name string
		text string
	}{
		{name: "empty"},
		{name: "old loose fixture", text: "idp_test"},
		{name: "missing prefix", text: "11111111111111111111111111111111"},
		{name: "short", text: "idp_1111"},
		{name: "uppercase hex", text: "idp_1111111111111111111111111111111A"},
		{name: "non hex", text: "idp_1111111111111111111111111111111z"},
		{name: "leading whitespace", text: " " + validNamespace},
		{name: "trailing whitespace", text: validNamespace + " "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if pattern.MatchString(test.text) {
				t.Fatalf("public pattern accepted %q", test.text)
			}
			if _, err := Parse(test.text); err == nil {
				t.Fatalf("Parse(%q) succeeded", test.text)
			}
		})
	}
}

func TestOptionalIdentityDistinguishesAbsenceFromMalformed(t *testing.T) {
	if value, present, err := ParseOptional(""); err != nil || present || value.String() != "" {
		t.Fatalf("ParseOptional(empty) = (%q, %t, %v)", value.String(), present, err)
	}
	if _, present, err := ParseOptional("idp_test"); err == nil || present {
		t.Fatalf("ParseOptional(invalid) = (present:%t, err:%v)", present, err)
	}
}
