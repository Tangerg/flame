package resourceid

import (
	"strings"
	"testing"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestOperationalIdentitiesAreExactBoundedAndDistinct(t *testing.T) {
	for _, parse := range []struct {
		name string
		call func(string) error
	}{
		{name: "session", call: func(value string) error { _, err := ParseSession(value); return err }},
		{name: "run", call: func(value string) error { _, err := ParseRun(value); return err }},
		{name: "segment", call: func(value string) error { _, err := ParseSegment(value); return err }},
		{name: "item", call: func(value string) error { _, err := ParseItem(value); return err }},
		{name: "schedule", call: func(value string) error { _, err := ParseSchedule(value); return err }},
	} {
		t.Run(parse.name, func(t *testing.T) {
			for _, invalid := range []string{
				"", " value", "value ", "val\nue", "value\x00", string([]byte{0xff}),
				strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1),
			} {
				if err := parse.call(invalid); err == nil {
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
