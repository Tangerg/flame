package runs

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestResumeRejectsStructurallyInvalidMediaBeforeConsumingWait(t *testing.T) {
	pending := testApprovalPending("member_1", time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))
	claims := 0
	effects := &fakeEffects{mutateClaim: func(*ExecutorCheckpoint) { claims++ }}
	sessions := &fakeRunSessions{
		sess: testsupport.MustRestoreSession(session.Snapshot{
			ID: "ses_1", Workspace: testsupport.MustWorkspace("/work"),
		}),
		pending: map[string]Pending{"run_1": pending},
	}
	control := &fakeExecutionPorts{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{block: true}, control, sessions, effects)
	command := ResumeCommand{
		RunID:              "run_1",
		CallerCapabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: interrupt.Approval,
			Approval: &ApprovalResponse{Approved: true},
		}},
		Input: []transcript.ContentBlock{{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: nil}},
	}
	for range 2 {
		result, err := coordinator.Resume(t.Context(), command)
		if !errors.Is(err, ErrUnsupportedMedia) {
			t.Fatalf("resume refusal = %v, want categorized model refusal", err)
		}
		if result.RunID != "" || claims != 0 || control.resumed || len(effects.openingSnapshot()) != 0 {
			t.Fatal("rejected input advanced the waiting execution")
		}
		if !reflect.DeepEqual(sessions.pending["run_1"], pending) {
			t.Fatal("rejected input consumed the original waiting state")
		}
	}
}
