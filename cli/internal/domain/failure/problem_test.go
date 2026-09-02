package failure

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestProblemOwnsAndPresentsRecoveryMetadata(t *testing.T) {
	t.Parallel()

	problems := []*protocol.ProblemData{
		{
			Type: protocol.ProblemRateLimited, Detail: "additional declarations required",
			DocURL: "https://docs.example/capabilities", RetryAfterSeconds: 2,
		},
		{
			Type:                 protocol.ErrCapabilityNotNeg.Error(),
			RequiredCapabilities: []protocol.CapabilityRequirement{{Type: protocol.RequirementFeature, Name: "subagents"}},
		},
		{
			Type:      protocol.ErrSessionHasActiveRun.Error(),
			ActiveRun: &protocol.ActiveRunRef{RunID: "run_1", Status: protocol.RunStatusWaiting},
		},
		{
			Type:   protocol.ErrInvalidParams.Error(),
			Errors: []protocol.FieldError{{Field: "features", Detail: "subagents is absent"}},
		},
	}
	var rendered []string
	for _, problem := range problems {
		if err := Validate(problem); err != nil {
			t.Fatal(err)
		}
		rendered = append(rendered, String(problem))
	}
	presentation := strings.Join(rendered, " ")
	for _, want := range []string{"additional declarations required", "retry after 2s", "docs.example", "feature:subagents", "run_1 (waiting)", "features: subagents is absent"} {
		if !strings.Contains(presentation, want) {
			t.Fatalf("String() omitted %q: %s", want, presentation)
		}
	}

	capabilitiesClone := Clone(problems[1])
	activeRunClone := Clone(problems[2])
	fieldsClone := Clone(problems[3])
	capabilitiesClone.RequiredCapabilities[0].Name = "mutated"
	activeRunClone.ActiveRun.RunID = "run_2"
	fieldsClone.Errors[0].Detail = "mutated"
	if Equal(problems[1], capabilitiesClone) || problems[1].RequiredCapabilities[0].Name != "subagents" ||
		problems[2].ActiveRun.RunID != "run_1" || problems[3].Errors[0].Detail != "subagents is absent" {
		t.Fatalf("Clone retained caller-owned state: sources=%+v", problems)
	}
	if !Equal(problems[0], Clone(problems[0])) || Clone(nil) != nil || !Equal(nil, nil) {
		t.Fatal("problem clone/equality identity is broken")
	}
}

func TestProblemRejectsMalformedStructuredLeaves(t *testing.T) {
	t.Parallel()

	tests := []protocol.ProblemData{
		{},
		{Type: "rate_limited", RetryAfterSeconds: -1},
		{Type: "capability_not_negotiated", RequiredCapabilities: []protocol.CapabilityRequirement{{Type: "unknown", Name: "x"}}},
		{Type: "capability_not_negotiated", RequiredCapabilities: []protocol.CapabilityRequirement{{Type: protocol.RequirementFeature}}},
		{Type: "capability_not_negotiated", RetryAfterSeconds: 2, RequiredCapabilities: []protocol.CapabilityRequirement{{Type: protocol.RequirementFeature, Name: "subagents"}}},
		{Type: "session_has_active_run", ActiveRun: &protocol.ActiveRunRef{Status: protocol.RunStatusRunning}},
		{Type: "session_has_active_run", ActiveRun: &protocol.ActiveRunRef{RunID: "run_1", Status: "queued"}},
		{Type: "invalid_params", Errors: []protocol.FieldError{{Field: "provider"}}},
	}
	for _, problem := range tests {
		if err := Validate(&problem); err == nil {
			t.Fatalf("Validate accepted malformed problem: %+v", problem)
		}
	}
}
