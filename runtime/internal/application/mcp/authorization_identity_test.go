package mcp

import (
	"regexp"
	"strings"
	"testing"
)

const testAuthorizationAttemptID = "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestAuthorizationAttemptIdentityRoundTrip(t *testing.T) {
	generated := newAuthorizationAttemptID()
	if !regexp.MustCompile(AuthorizationAttemptIDPattern).MatchString(generated.String()) {
		t.Fatalf("public pattern rejects generated identity %q", generated.String())
	}
	parsed, err := ParseAuthorizationAttemptID(generated.String())
	if err != nil || parsed != generated {
		t.Fatalf("Parse(generated) = (%q, %v), want %q", parsed.String(), err, generated.String())
	}
}

func TestAuthorizationAttemptIdentityRejectsNonCanonicalMaterial(t *testing.T) {
	pattern := regexp.MustCompile(AuthorizationAttemptIDPattern)
	tests := []struct {
		name string
		text string
	}{
		{name: "empty"},
		{name: "old loose fixture", text: "mcpauth_test"},
		{name: "missing frame", text: strings.Repeat("A", minimumAuthorizationAttemptEntropyBytes)},
		{name: "short entropy", text: authorizationAttemptIDPrefix + strings.Repeat("A", minimumAuthorizationAttemptEntropyBytes-1)},
		{name: "long entropy", text: authorizationAttemptIDPrefix + strings.Repeat("A", maximumAuthorizationAttemptEntropyBytes+1)},
		{name: "lowercase entropy", text: authorizationAttemptIDPrefix + strings.Repeat("a", minimumAuthorizationAttemptEntropyBytes)},
		{name: "digit outside base32", text: authorizationAttemptIDPrefix + strings.Repeat("A", minimumAuthorizationAttemptEntropyBytes-1) + "8"},
		{name: "whitespace", text: testAuthorizationAttemptID + " "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if pattern.MatchString(test.text) {
				t.Fatalf("public pattern accepted %q", test.text)
			}
			if _, err := ParseAuthorizationAttemptID(test.text); err == nil {
				t.Fatalf("Parse(%q) succeeded", test.text)
			}
		})
	}
}
