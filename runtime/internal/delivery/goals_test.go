package delivery

import (
	"errors"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestMapGoalErrPreservesInvalidModelIdentity(t *testing.T) {
	err := mapGoalErr(modelchoice.ErrModelIdentity)
	if !errors.Is(err, protocol.ErrInvalidParams) || !errors.Is(err, modelchoice.ErrModelIdentity) {
		t.Fatalf("err = %v, want ErrInvalidParams and ErrModelIdentity", err)
	}
}

func TestGoalProjectsEveryMachineReadableReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domain goal.ReasonCode
		status goal.Status
		detail string
		wire   protocol.GoalReason
	}{
		{goal.ReasonStoppedByUser, goal.StatusPaused, "", protocol.GoalReason{Code: protocol.GoalReasonStoppedByUser}},
		{goal.ReasonRuntimeRestarted, goal.StatusPaused, "", protocol.GoalReason{Code: protocol.GoalReasonRuntimeRestarted}},
		{goal.ReasonRunStartFailed, goal.StatusPaused, "", protocol.GoalReason{Code: protocol.GoalReasonRunStartFailed}},
		{goal.ReasonAwaitingInput, goal.StatusPaused, "", protocol.GoalReason{Code: protocol.GoalReasonAwaitingInput}},
		{goal.ReasonTerminalOutcomeMissing, goal.StatusPaused, "", protocol.GoalReason{Code: protocol.GoalReasonTerminalOutcomeMissing}},
		{goal.ReasonRunNotCompleted, goal.StatusPaused, "failed", protocol.GoalReason{Code: protocol.GoalReasonRunNotCompleted, RunOutcome: protocol.OutcomeFailed}},

		{goal.ReasonBlockedByModel, goal.StatusBlocked, "safe context", protocol.GoalReason{Code: protocol.GoalReasonBlockedByModel, Explanation: "safe context"}},
	}

	for _, test := range tests {
		t.Run(string(test.domain), func(t *testing.T) {
			t.Parallel()
			value := serverGoalWithState(t, test.status, test.domain, test.detail)
			presented, err := presentGoal(value)
			if err != nil {
				t.Fatalf("presentGoal: %v", err)
			}
			if presented.Reason == nil || *presented.Reason != test.wire {
				t.Fatalf("reason = %+v, want %+v", presented.Reason, test.wire)
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
	selection, err := modelchoice.New("provider", "model")
	if err != nil {
		t.Fatal(err)
	}
	var outcome run.Outcome
	var explanation string
	switch reason {
	case goal.ReasonRunNotCompleted:
		outcome = run.Outcome(detail)
	case goal.ReasonBlockedByModel:
		explanation = detail
	}
	value, err := goal.Restore(goal.Snapshot{
		ReasonRunOutcome: outcome, ReasonExplanation: explanation,
		SessionID: "session-1", Objective: "finish the migration", Status: status,
		ReasonCode: reason, ModelSelection: selection,
		Capabilities: run.Capabilities{}, IncarnationID: "incarnation-1", Revision: 1,
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
