package goalref

import (
	"strings"
	"testing"
)

func TestParseIncarnationPreservesExactIdentity(t *testing.T) {
	const identity = "incarnation-ABC_123"
	parsed, err := ParseIncarnation(identity)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.String() != identity {
		t.Fatalf("identity = %q, want %q", parsed.String(), identity)
	}
}

func TestParseIncarnationRejectsMalformedIdentity(t *testing.T) {
	tests := []struct {
		name     string
		identity string
	}{
		{name: "empty"},
		{name: "leading whitespace", identity: " incarnation"},
		{name: "interior whitespace", identity: "incar nation"},
		{name: "non-printing", identity: "incarnation\u200b"},
		{name: "invalid UTF-8", identity: string([]byte{0xff})},
		{name: "too long", identity: strings.Repeat("界", MaximumIncarnationCharacters+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseIncarnation(test.identity); err == nil {
				t.Fatalf("ParseIncarnation(%q) succeeded", test.identity)
			}
		})
	}
}

func TestParseOptionalIncarnationDistinguishesAbsence(t *testing.T) {
	parsed, present, err := ParseOptionalIncarnation("")
	if err != nil || present || parsed.String() != "" {
		t.Fatalf("empty optional = (%q, %t, %v)", parsed.String(), present, err)
	}
	parsed, present, err = ParseOptionalIncarnation("incarnation")
	if err != nil || !present || parsed.String() != "incarnation" {
		t.Fatalf("present optional = (%q, %t, %v)", parsed.String(), present, err)
	}
}
