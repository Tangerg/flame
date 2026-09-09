package cmd

import (
	"strings"
	"testing"
)

// TestPreconditionFlagsAreRequired covers the constraint, not the call that
// declares it. A revision is an optimistic-concurrency precondition: losing it
// turns a compare-and-set update into an unconditional one, which is exactly
// the lost update the revision exists to prevent. The approvals scope is what
// keeps a listing from crossing sessions.
func TestPreconditionFlagsAreRequired(t *testing.T) {
	for name, invocation := range map[string][]string{
		"sessions update": {"sessions", "update", "ses_demo_1", "--title", "renamed"},
		"sessions rename": {"sessions", "rename", "ses_demo_1", "renamed"},
		"approvals list":  {"approvals", "list"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := executeCommand(t, nil, "", invocation...)
			if err == nil {
				t.Fatal("command ran without its required precondition flag")
			}
			if !strings.Contains(err.Error(), "required flag") {
				t.Fatalf("error = %v, want a required-flag refusal", err)
			}
		})
	}
}
