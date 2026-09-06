package runs

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestResumeActivationFailureSettlesAcceptedToolApproval(t *testing.T) {
	for _, decision := range []approval.Decision{approval.Allow, approval.Deny} {
		t.Run(string(decision), func(t *testing.T) {
			createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
			pending := testApprovalPending("member_1", createdAt)
			effects := &fakeEffects{}
			sessions := &fakeRunSessions{
				sess: testsupport.MustRestoreSession(session.Snapshot{
					ID: "ses_1", Workspace: testsupport.MustWorkspace("/work"),
				}),
				pending: map[string]Pending{"run_1": pending},
			}
			control := &fakeExecutionPorts{resumeErr: errors.New("activate continuation failed")}
			coordinator := newUseCaseCoordinator(&fakeExecutor{block: true}, control, sessions, effects)
			result, err := coordinator.Resume(t.Context(), ResumeCommand{
				RunID:              "run_1",
				CallerCapabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
				Responses: []ResumeResponse{{
					ItemID: "item_1", Kind: ApprovalResponseKind,
					Approval: &ApprovalResponse{Approved: decision == approval.Allow},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			var settled []transcript.Item
			events := collectEvents(result.Events)
			for _, event := range events {
				if completed, ok := event.Payload.(ItemCompleted); ok {
					settled = append(settled, completed.Item)
				}
			}
			terminal, ok := events[len(events)-1].Payload.(SegmentFinished)
			if !ok || !runHasOutcome(terminal.Run, run.OutcomeFailed) {
				t.Fatalf("activation failure terminal = %+v", events[len(events)-1].Payload)
			}
			if len(settled) != 1 || settled[0].ID() != "item_1" ||
				settled[0].Status() != transcript.ItemIncomplete || settled[0].ApprovalDecision() != decision ||
				!settled[0].OccurredAt().Equal(pending.Interrupts[0].ItemOccurredAt) {
				t.Fatalf("accepted approval left unsettled after activation failure: %+v", settled)
			}
		})
	}
}
