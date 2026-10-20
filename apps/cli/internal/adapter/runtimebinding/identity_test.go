package runtimebinding

import (
	"strings"
	"testing"
)

func TestRequireIdentityOwnsPresenceAndExpectation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		actual   string
		expected string
		want     string
	}{
		{name: "minted", actual: "sch_1"},
		{name: "referenced", actual: "sch_1", expected: "sch_1"},
		{name: "minted without an id", want: "create schedule returned a result without an id"},
		{name: "referenced without an id", expected: "sch_1", want: "create schedule returned a result without an id"},
		{name: "other id", actual: "sch_2", expected: "sch_1", want: `create schedule returned id "sch_2" for "sch_1"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := requireIdentity("create schedule", test.actual, test.expected)
			if test.want == "" {
				if err != nil {
					t.Fatalf("requireIdentity = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("requireIdentity error = %v, want %q", err, test.want)
			}
			requireRuntimeContractViolation(t, err)
		})
	}
}
