package runs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
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
				CallerCapabilities: run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptApproval}},
				Responses: []ResumeResponse{{
					ItemID: "item_1", Kind: run.InterruptApproval,
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
				!settled[0].OccurredAt().Equal(fixtureItems(pending)["item_1"].OccurredAt()) {
				t.Fatalf("accepted approval left unsettled after activation failure: %+v", settled)
			}
		})
	}
}

func TestResumeRestorationAndObservationFailuresBelongToAcceptedRun(t *testing.T) {
	for _, phase := range []string{"restore", "observe"} {
		t.Run(phase, func(t *testing.T) {
			pending := testApprovalPending("member_1", time.Date(2025, 1, 2, 1, 0, 0, 0, time.UTC))
			effects := &fakeEffects{}
			sessions := &fakeRunSessions{sess: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}), pending: map[string]Pending{"run_1": pending}}
			failure := errors.New("accepted " + phase + " failed")
			control := &fakeExecutionPorts{}
			executor := &fakeExecutor{block: true}
			if phase == "restore" {
				control.rehydrateErr = failure
			} else {
				executor.startErr = failure
			}
			coordinator := newUseCaseCoordinator(executor, control, sessions, effects)
			control.stageContinuationCheck = func() {
				if len(effects.openingSnapshot()) != 1 {
					t.Error("restoration preceded acceptance")
				}
				if _, found := coordinator.registry.Get("run_1"); !found {
					t.Error("accepted restoration has no Run owner")
				}
			}
			result, err := coordinator.Resume(t.Context(), ResumeCommand{RunID: "run_1", CallerCapabilities: run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptApproval}}, Responses: []ResumeResponse{{ItemID: "item_1", Kind: run.InterruptApproval, Approval: &ApprovalResponse{Approved: true}}}})
			if err != nil || result.SegmentID == "" {
				t.Fatalf("accepted restoration failure rejected the command: %+v,%v", result, err)
			}
			events := collectEvents(result.Events)
			terminal, ok := events[len(events)-1].Payload.(SegmentFinished)
			if !ok || !runHasOutcome(terminal.Run, run.OutcomeFailed) || events[len(events)-1].SegmentID != result.SegmentID || control.resumed {
				t.Fatalf("accepted failure did not settle its exact Segment: %+v", events)
			}
			if len(effects.openingSnapshot()) != 1 || len(control.released) != 1 {
				t.Fatalf("opening or release repeated: openings=%d release=%v", len(effects.openingSnapshot()), control.released)
			}
			requireCoordinatorShutdown(t, coordinator)
		})
	}
}

func TestResumeExposesAcceptedOwnerWhileRestorationIsBlocked(t *testing.T) {
	pending := testApprovalPending("member_root", time.Date(2025, 1, 2, 1, 0, 0, 0, time.UTC))
	sessions := &fakeRunSessions{sess: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}), pending: map[string]Pending{"run_1": pending}}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	control := &fakeExecutionPorts{stageContinuationCheck: func() { close(entered); <-release }}
	effects := &fakeEffects{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{block: true}, control, sessions, effects)
	result, err := coordinator.Resume(t.Context(), ResumeCommand{RunID: "run_1", CallerCapabilities: run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptApproval}}, Responses: []ResumeResponse{{ItemID: "item_1", Kind: run.InterruptApproval, Approval: &ApprovalResponse{Approved: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	resumed, _ := effects.opening().Resume()
	for _, replacement := range resumed.Runs {
		value := replacement.State()
		coordinator.runs.(*fakeRunProjection).runs[value.ID()] = value
	}
	delete(sessions.pending, "run_1")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := coordinator.Subscribe(ctx, SubscribeRequest{RunID: result.RunID, SegmentID: result.SegmentID, CallerCapabilities: run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptApproval}}}); err != nil {
		t.Fatalf("accepted restoration has no observable owner: %v", err)
	}
	if control.resumed {
		t.Fatal("blocked restoration delivered an answer")
	}
	steerCtx, cancelSteer := context.WithCancel(t.Context())
	cancelSteer()
	if _, err := coordinator.Steer(steerCtx, SteerCommand{RunID: result.RunID, ExpectedSegmentID: result.SegmentID, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "steer after activation"}}}); !errors.Is(err, context.Canceled) || len(control.steered) != 0 {
		t.Fatalf("steer reached the unrestored executor: %v", err)
	}
	type cancelResult struct {
		result CancelResult
		err    error
	}
	settled := make(chan cancelResult, 1)
	go func() {
		result, err := coordinator.Cancel(t.Context(), CancelCommand{RunID: result.RunID, Reason: "stop after restoration"})
		settled <- cancelResult{result: result, err: err}
	}()
	select {
	case outcome := <-settled:
		t.Fatalf("cancel crossed blocked restoration: %+v,%v", outcome.result, outcome.err)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	outcome := <-settled
	if outcome.err != nil || outcome.result.Run.State() != run.Canceled {
		t.Fatalf("cancel across restoration=%+v,%v", outcome.result, outcome.err)
	}
	consumeEvents(result.Events)
	requireCoordinatorShutdown(t, coordinator)
}
