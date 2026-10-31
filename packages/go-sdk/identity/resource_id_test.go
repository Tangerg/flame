package identity

import (
	"regexp"
	"strings"
	"testing"
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
				strings.Repeat("界", MaximumResourceCharacters+1),
			} {
				if err := rule.call(invalid); err == nil {
					t.Errorf("accepted %q", invalid)
				}
			}
		})
	}

	if err := ValidateSession(strings.Repeat("界", MaximumResourceCharacters)); err != nil {
		t.Fatalf("boundary Session identity: %v", err)
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

func TestInstallationHostKeyRoundTrips(t *testing.T) {
	id, err := ParseInstallation("12345678-1234-1234-1234-123456789abc")
	if err != nil {
		t.Fatal(err)
	}
	if back, err := ParseHostInstallation(id.Host()); err != nil || back != id {
		t.Fatalf("host key round trip = %v, %v", back, err)
	}
	if _, err := ParseHostInstallation("notes"); err == nil {
		t.Fatal("a host key outside Flame's identity space was accepted")
	}
}
