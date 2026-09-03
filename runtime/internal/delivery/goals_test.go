package delivery

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestMapGoalErrPreservesInvalidModelIdentity(t *testing.T) {
	err := mapGoalErr(modelref.ErrModelIdentity, "goals.start")
	if !errors.Is(err, protocol.ErrInvalidParams) || !errors.Is(err, modelref.ErrModelIdentity) {
		t.Fatalf("err = %v, want ErrInvalidParams and ErrModelIdentity", err)
	}
}

func TestGoalProjectsEveryMachineReadableReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domain goal.ReasonCode
		status goal.Status
		detail string
		wire   protocol.GoalReasonCode
	}{
		{goal.ReasonStoppedByUser, goal.StatusPaused, "", protocol.GoalReasonStoppedByUser},
		{goal.ReasonRuntimeRestarted, goal.StatusPaused, "", protocol.GoalReasonRuntimeRestarted},
		{goal.ReasonRunStartFailed, goal.StatusPaused, "", protocol.GoalReasonRunStartFailed},
		{goal.ReasonAwaitingInput, goal.StatusPaused, "", protocol.GoalReasonAwaitingInput},
		{goal.ReasonTerminalOutcomeMissing, goal.StatusPaused, "", protocol.GoalReasonTerminalOutcomeMissing},
		{goal.ReasonRunNotCompleted, goal.StatusPaused, "failed", protocol.GoalReasonRunNotCompleted},
		{goal.ReasonRunBudgetReached, goal.StatusBlocked, "", protocol.GoalReasonRunBudgetReached},
		{goal.ReasonCostBudgetReached, goal.StatusBlocked, "", protocol.GoalReasonCostBudgetReached},
		{goal.ReasonStepBudgetReached, goal.StatusBlocked, "", protocol.GoalReasonStepBudgetReached},
		{goal.ReasonPricingUnavailable, goal.StatusBlocked, "", protocol.GoalReasonPricingUnavailable},
		{goal.ReasonBlockedByModel, goal.StatusBlocked, "safe context", protocol.GoalReasonBlockedByModel},
	}

	for _, test := range tests {
		t.Run(string(test.domain), func(t *testing.T) {
			t.Parallel()
			value := serverGoalWithState(t, test.status, test.domain, test.detail)
			presented, err := presentGoal(value)
			if err != nil {
				t.Fatalf("presentGoal: %v", err)
			}
			if presented.Reason == nil || presented.Reason.Code != test.wire || presented.Reason.Detail != test.detail {
				t.Fatalf("reason = %+v, want code %q detail %q", presented.Reason, test.wire, test.detail)
			}
		})
	}
}

func TestGoalOmitsReasonForActiveGoal(t *testing.T) {
	t.Parallel()

	presented, err := presentGoal(serverGoalWithState(t, goal.StatusActive, goal.ReasonNone, ""))
	if err != nil {
		t.Fatalf("presentGoal: %v", err)
	}
	if presented.Reason != nil {
		t.Fatalf("active reason = %+v, want nil", presented.Reason)
	}
}

func TestGoalProjectsCompletingStateDuringSettlement(t *testing.T) {
	t.Parallel()

	presented, err := presentGoal(serverGoalWithState(t, goal.StatusComplete, goal.ReasonNone, ""))
	if err != nil {
		t.Fatalf("presentGoal: %v", err)
	}
	if presented.Status != protocol.GoalCompleting {
		t.Fatalf("status = %q, want %q", presented.Status, protocol.GoalCompleting)
	}
}

func serverGoalWithState(t *testing.T, status goal.Status, reason goal.ReasonCode, detail string) goal.Goal {
	t.Helper()
	selection, err := modelref.New("provider", "model")
	if err != nil {
		t.Fatal(err)
	}
	value, err := goal.Restore(goal.Snapshot{
		SessionID: "session-1", Objective: "finish the migration", Status: status,
		ReasonCode: reason, ReasonDetail: detail, ModelSelection: selection,
		Capabilities: run.Capabilities{}, Budget: goal.UnlimitedBudget(), IncarnationID: "incarnation-1", Revision: 1,
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
