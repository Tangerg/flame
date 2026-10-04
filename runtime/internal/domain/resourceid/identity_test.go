package resourceid

import (
	"regexp"
	"strings"
	"testing"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestOperationalIdentitiesAreExactBoundedAndDistinct(t *testing.T) {
	for _, rule := range []struct {
		name string
		call func(string) error
	}{
		{name: "session", call: ValidateSession},
		{name: "run", call: ValidateRun},
		{name: "segment", call: ValidateSegment},
		{name: "item", call: ValidateItem},
	} {
		t.Run(rule.name, func(t *testing.T) {
			for _, invalid := range []string{
				"", " value", "value ", "val\nue", "value\x00", string([]byte{0xff}),
				strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1),
			} {
				if err := rule.call(invalid); err == nil {
					t.Errorf("accepted %q", invalid)
				}
			}
		})
	}

	session, err := ParseSession(strings.Repeat("界", runtimeidentity.MaximumResourceCharacters))
	if err != nil || session.String() == "" {
		t.Fatalf("boundary Session identity = %q, %v", session.String(), err)
	}
	if err := (SessionID{}).Validate(); err == nil {
		t.Fatal("zero Session identity is valid")
	}
}

func TestInstallationIdentityHasOneNonzeroWireGrammar(t *testing.T) {
	pattern := regexp.MustCompile(InstallationIDPattern())
	for _, test := range []struct {
		text  string
		valid bool
	}{
		{"12345678-1234-1234-1234-123456789abc", true},
		{"00000000-0000-0000-0000-000000000001", true},
		{"00000000-0000-0000-0000-000000000000", false},
		{"12345678-1234-1234-1234-123456789ABC", false},
		{"12345678123412341234123456789abc", false},
	} {
		_, err := ParseInstallation(test.text)
		if (err == nil) != test.valid || pattern.MatchString(test.text) != test.valid {
			t.Fatalf("installation grammar disagrees for %q: %v", test.text, err)
		}
	}
}
