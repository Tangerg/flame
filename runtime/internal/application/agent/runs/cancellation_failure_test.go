package runs

import (
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestLiveChildCancellationReleasesClaimWhenExecutorTeardownFails(t *testing.T) {
	plan := runningChildCancellationPlan()
	plan.executor = ExecutorRef{SessionID: "session", ExecutorID: "turn_root"}
	teardownErr := errors.New("subtree teardown failed")
	control := &fakeExecutionPorts{
		cancelSubtree: func(ref ExecutorRef, memberID, reason string) error {
			if ref != (ExecutorRef{SessionID: "session", ExecutorID: "turn_root"}) {
				t.Fatalf("CancelSubtree execution = %+v, want session/exec_root", ref)
			}
			if memberID != plan.target.memberID {
				t.Fatalf(
					"CancelSubtree member = %q, want %q",
					memberID,
					plan.target.memberID,
				)
			}
			if reason != "stop child" {
				t.Fatalf("CancelSubtree reason = %q, want stop child", reason)
			}
			return teardownErr
		},
	}
	coordinator := mustNewCoordinator(Dependencies{RunningSubtreeCanceler: control})
	live := testRunTreeOwner(t, nil)
	if _, err := live.beginExecution(t.Context(), nil); err != nil {
		t.Fatal(err)
	}

	_, err := coordinator.cancelLiveChild(
		CancelCommand{
			RunID:         plan.target.run.ID(),
			Reason:        "stop child",
			AllowChildRun: true,
		},
		plan,
		live,
	)
	if !errors.Is(err, teardownErr) {
		t.Fatalf("cancel live child error = %v, want teardown cause", err)
	}
	for _, identity := range []string{plan.target.run.ID(), plan.target.memberID} {
		if !strings.Contains(err.Error(), identity) {
			t.Fatalf("cancel live child error = %q, want identity %q", err, identity)
		}
	}
	if live.childCancel != nil {
		t.Fatal("executor teardown failure retained the child cancellation claim")
	}
	retry, err := live.beginChildCancellation(plan, "retry child")
	if err != nil {
		t.Fatalf("retry child cancellation claim: %v", err)
	}
	live.abortChildCancellation(retry, errors.New("test cleanup"))
}

func TestLiveRootCancellationRetainsFailedSettlement(t *testing.T) {
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{block: true, releaseErr: cleanupErr}
	coordinator := testCoordinator(executor, &fakeEffects{})
	coordinator.rootCancellation = executor
	sessions := &fakeRunSessions{}
	coordinator.sessionReader = sessions
	coordinator.sessionCreator = sessions
	coordinator.activeRuns = sessions
	coordinator.interrupts = sessions
	coordinator.terminations = sessions
	coordinator.runs = &fakeRunProjection{runs: map[string]run.Run{"run_1": runForSegment(testSegment())}}
	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatal(err)
	}
	next, stop := iter.Pull(stream)
	defer stop()
	next()
	_, err = coordinator.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop"})
	if _, failed := errors.AsType[*CancellationSettlementError](err); !failed || !errors.Is(err, cleanupErr) {
		t.Fatalf("cancel erased incomplete settlement: %v", err)
	}
	consumePulledEvents(next)
	if executor.releases() != 1 {
		t.Fatalf("release count=%d, want one", executor.releases())
	}
}
