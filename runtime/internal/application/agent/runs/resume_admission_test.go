package runs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type consumedBeforeAdmissionProjection struct {
	Projection
	consume func()
}

func (r *consumedBeforeAdmissionProjection) Run(ctx context.Context, id string) (run.Run, bool, error) {
	value, found, err := r.Projection.Run(ctx, id)
	if err == nil && found && r.consume != nil {
		consume := r.consume
		r.consume = nil
		consume()
	}
	return value, found, err
}

func TestResumeRejectsAHandoffConsumedBeforeAdmission(t *testing.T) {
	pending := testApprovalPending("member_1", time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC))
	pending.Interrupts = []OpenInterrupt{{ItemID: "item_1"}}
	pending.Bindings[0].ToolCallID = ""
	sessions := &fakeRunSessions{
		sess:    testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}),
		pending: map[string]Pending{"run_1": pending},
	}
	claims := 0
	effects := &fakeEffects{mutateClaim: func(*ExecutorCheckpoint) { claims++ }}
	control := &fakeExecutionPorts{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, effects)
	projection := coordinator.runs.(*fakeRunProjection)
	coordinator.runs = &consumedBeforeAdmissionProjection{
		Projection: projection,
		consume: func() {
			lease, acquired, err := coordinator.admission.AcquireRun(t.Context(), "ses_1", "/work")
			if err != nil || !acquired {
				t.Fatalf("competing resume admission = %t, %v", acquired, err)
			}
			defer lease.Release()
			items := coordinator.items.(*fakeItemProjection)
			answered, err := items.items["item_1"].AnswerQuestion([][]string{{"Yes"}})
			if err != nil {
				t.Fatal(err)
			}
			items.items["item_1"] = answered
			delete(sessions.pending, "run_1")
			continued, err := projection.runs["run_1"].Resume("seg_winner", pending.CreatedAt.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			finished, err := continued.Terminate(run.Termination{Outcome: run.OutcomeCompleted, FinishedAt: pending.CreatedAt.Add(2 * time.Second)})
			if err != nil {
				t.Fatal(err)
			}
			projection.runs["run_1"] = finished
		},
	}
	_, err := coordinator.Resume(t.Context(), ResumeCommand{
		RunID:              "run_1",
		CallerCapabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
		Responses:          []ResumeResponse{{ItemID: "item_1", Kind: interrupt.Question, Question: &QuestionResponse{Answers: [][]string{{"Yes"}}}}},
	})
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("resume of a consumed handoff = %v, want ErrInterruptNotOpen", err)
	}
	if claims != 0 || control.resumed || len(effects.openingSnapshot()) != 0 {
		t.Fatal("a consumed handoff opened another continuation")
	}
	lease, acquired, err := coordinator.admission.AcquireRun(t.Context(), "ses_1", "/work")
	if err != nil || !acquired {
		t.Fatalf("refused resume retained admission: %t, %v", acquired, err)
	}
	lease.Release()
}
