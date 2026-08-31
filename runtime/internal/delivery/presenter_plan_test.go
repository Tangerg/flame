package delivery

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/protocol"
)

// TestPlanUpdateCarriesTheCompletePlan proves the first-class run event carries the
// same revisioned Plan shape that plan.get returns.
func TestPlanUpdateCarriesTheCompletePlan(t *testing.T) {
	t.Parallel()

	event := presentRunEvent(runs.PlanSnapshot{
		SessionID: "ses_1", Revision: 2, UpdatedAt: time.Unix(9, 0).UTC(),
		Steps: []plan.Step{{
			Description: "read the contract", Status: plan.StatusInProgress,
		}, {
			Description: "write the fixture", Status: plan.StatusPending,
		}},
	})

	if event.Type != protocol.StreamPlanUpdated || event.Plan == nil {
		t.Fatalf("event = %+v, want plan.updated with a Plan", event)
	}
	if event.Plan.SessionID != "ses_1" || event.Plan.State == nil || event.Plan.State.Revision != 2 ||
		len(event.Plan.State.Steps) != 2 || event.Plan.State.Steps[0].Status != protocol.PlanStatusInProgress {
		t.Fatalf("Plan = %+v, want the complete revisioned replacement", event.Plan)
	}
}

// TestPlanQueryAnswersWithTheStreamsOwnSnapshot proves
// plan_revision_never_goes_backwards at the wire boundary.
//
// plan.get is the Plan's cold recovery source, so it is what a client calls when
// it missed the events — after a reload, a rollback, or a replay window it could not
// reach. The answer therefore has to be foldable by the SAME rule the stream is
// folded by: same shape, same key, and the store's own committed state rather
// than a re-derived value. Collapsing a committed state to absence would discard
// the recovery source and leave the panel stale in exactly the situation recovery
// exists for.
func TestPlanQueryAnswersWithTheStreamsOwnSnapshot(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := insertSessionFixture(ctx, rt.sess, "recovering", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if saveTestPlanErr := saveTestPlan(ctx, rt.plan, ses.ID(), []plan.Step{{Description: "first", Status: plan.StatusCompleted}}); saveTestPlanErr != nil {
		t.Fatalf("seed plan: %v", saveTestPlanErr)
	}

	first, err := s.GetPlan(ctx, protocol.GetPlanRequest{SessionID: ses.ID()})
	if err != nil {
		t.Fatalf("plan.get: %v", err)
	}
	stored, err := rt.plan.State(ctx, ses.ID())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if first.SessionID != ses.ID() {
		t.Fatalf("cold read = %+v, want the Plan for %s", first, ses.ID())
	}
	storedState, committed := stored.State()
	if !committed || first.State == nil || first.State.Revision != storedState.Revision() {
		t.Fatalf("cold read state = %+v, want committed state %+v", first.State, storedState)
	}
	if len(first.State.Steps) != 1 || first.State.Steps[0].Description != "first" {
		t.Fatalf("cold read list = %+v, want the stored list", first.State.Steps)
	}

	if saveTestPlanErr := saveTestPlan(ctx, rt.plan, ses.ID(), []plan.Step{{Description: "second", Status: plan.StatusInProgress}}); saveTestPlanErr != nil {
		t.Fatalf("advance plan: %v", saveTestPlanErr)
	}
	second, err := s.GetPlan(ctx, protocol.GetPlanRequest{SessionID: ses.ID()})
	if err != nil {
		t.Fatalf("plan.get again: %v", err)
	}
	if second.State == nil || second.State.Revision <= first.State.Revision {
		t.Fatalf("state went from %+v to %+v; a later read must never answer older", first.State, second.State)
	}
}

func TestUnwrittenAndExplicitlyClearedPlansStayDistinctOnTheWire(t *testing.T) {
	t.Parallel()

	unwritten := presentStoredPlan("ses_1", plan.Current{})
	if unwritten.SessionID != "ses_1" || unwritten.State != nil {
		t.Fatalf("unwritten Plan = %+v, want identity with no committed state", unwritten)
	}

	cleared, err := (plan.Current{}).Replace(nil, time.Unix(3, 0).UTC())
	if err != nil {
		t.Fatalf("clear Plan: %v", err)
	}
	current, err := plan.CurrentOf(cleared)
	if err != nil {
		t.Fatalf("own cleared Plan: %v", err)
	}
	presented := presentStoredPlan("ses_1", current)
	if presented.State == nil || presented.State.Revision != 1 ||
		presented.State.Steps == nil || len(presented.State.Steps) != 0 {
		t.Fatalf("cleared Plan = %+v, want committed empty replacement", presented)
	}
}
