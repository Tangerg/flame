package identity

import (
	"regexp"
	"testing"
)

const validNamespace = "idp_11111111111111111111111111111111"

func TestIdempotencyNamespaceRoundTrip(t *testing.T) {
	if !regexp.MustCompile(IdempotencyNamespacePattern).MatchString(validNamespace) {
		t.Fatalf("public pattern rejects canonical namespace %q", validNamespace)
	}
	parsed, err := ParseIdempotencyNamespace(validNamespace)
	if err != nil || parsed.String() != validNamespace {
		t.Fatalf("Parse(%q) = (%q, %v)", validNamespace, parsed.String(), err)
	}
}

func TestParseRejectsNonCanonicalMaterial(t *testing.T) {
	pattern := regexp.MustCompile(IdempotencyNamespacePattern)
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
			if _, err := ParseIdempotencyNamespace(test.text); err == nil {
				t.Fatalf("Parse(%q) succeeded", test.text)
			}
		})
	}
}

func TestOptionalIdentityDistinguishesAbsenceFromMalformed(t *testing.T) {
	if value, present, err := ParseOptionalIdempotencyNamespace(""); err != nil || present || value.String() != "" {
		t.Fatalf("ParseOptionalIdempotencyNamespace(empty) = (%q, %t, %v)", value.String(), present, err)
	}
	if _, present, err := ParseOptionalIdempotencyNamespace("idp_test"); err == nil || present {
		t.Fatalf("ParseOptionalIdempotencyNamespace(invalid) = (present:%t, err:%v)", present, err)
	}
}
