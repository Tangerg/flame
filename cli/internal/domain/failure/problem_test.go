package failure

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestProblemOwnsAndPresentsRecoveryMetadata(t *testing.T) {
	t.Parallel()

	problem := &Problem{
		Type: "capability_not_negotiated", Detail: "additional declarations required",
		DocURL: "https://docs.example/capabilities", RetryAfterSeconds: 2,
		RequiredCapabilities: []protocol.CapabilityRequirement{{Type: protocol.RequirementFeature, Name: "subagents"}},
		ActiveRun:            &protocol.ActiveRunRef{RunID: "run_1", Status: protocol.RunStatusWaiting},
		Errors:               []protocol.FieldError{{Field: "features", Detail: "subagents is absent"}},
	}
	if err := problem.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"additional declarations required", "retry after 2s", "docs.example", "feature:subagents", "run_1 (waiting)", "features: subagents is absent"} {
		if !strings.Contains(problem.String(), want) {
			t.Fatalf("Problem.String() omitted %q: %s", want, problem)
		}
	}

	clone := problem.Clone()
	clone.RequiredCapabilities[0].Name = "mutated"
	clone.ActiveRun.RunID = "run_2"
	clone.Errors[0].Detail = "mutated"
	if problem.Equal(clone) || problem.RequiredCapabilities[0].Name != "subagents" || problem.ActiveRun.RunID != "run_1" || problem.Errors[0].Detail != "subagents is absent" {
		t.Fatalf("Clone retained caller-owned state: source=%+v clone=%+v", problem, clone)
	}
	if !problem.Equal(problem.Clone()) || (*Problem)(nil).Clone() != nil || !(*Problem)(nil).Equal(nil) {
		t.Fatal("problem clone/equality identity is broken")
	}
}

func TestProblemRejectsMalformedStructuredLeaves(t *testing.T) {
	t.Parallel()

	tests := []Problem{
		{},
		{Type: "rate_limited", RetryAfterSeconds: -1},
		{Type: "capability_not_negotiated", RequiredCapabilities: []protocol.CapabilityRequirement{{Type: "unknown", Name: "x"}}},
		{Type: "capability_not_negotiated", RequiredCapabilities: []protocol.CapabilityRequirement{{Type: protocol.RequirementFeature}}},
		{Type: "session_has_active_run", ActiveRun: &protocol.ActiveRunRef{Status: protocol.RunStatusRunning}},
		{Type: "session_has_active_run", ActiveRun: &protocol.ActiveRunRef{RunID: "run_1", Status: "queued"}},
		{Type: "invalid_params", Errors: []protocol.FieldError{{Field: "provider"}}},
	}
	for _, problem := range tests {
		if err := problem.Validate(); err == nil {
			t.Fatalf("Validate accepted malformed problem: %+v", problem)
		}
	}
}
