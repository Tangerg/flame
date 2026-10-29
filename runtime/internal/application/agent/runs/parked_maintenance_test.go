package runs

import (
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestCancellationDuringParkedMaintenanceIsBusyUntilOwnerReleases(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	releaseFinish := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseFinish)
	effects := &fakeEffects{finishStarted: started, finishRelease: release}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{{
		Member: ExecutorMember{MemberID: "member_root"},
		Payload: mustTreeInterrupted(t, testExecutorCheckpoint(), []MemberInterruption{{
			MemberID: "member_root", RequestID: "request_question",
			Interrupt: treeBarrierQuestion("Continue?"),
		}}),
	}}}
	sessions := &fakeRunSessions{pending: make(map[string]Pending)}
	projection := &fakeRunProjection{runs: make(map[string]run.Run)}
	coordinator := mustNewCoordinator(Dependencies{
		Observations: executor, Releases: &fakeExecutionPorts{},
		Projection: testProjectionPorts(effects), Session: testSessionPorts(sessions),
		Runs: projection, Admissions: testsupport.NewAdmissionGate(),
	})
	spec := testSegment()
	spec.Capabilities = run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}}
	stream, err := coordinator.openSegment(t.Context(), testAdmittedSegment(t, coordinator, spec))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseFinish(); consumeEvents(stream) })
	<-started
	barriers := effects.barrierSnapshot()
	if len(barriers) != 1 {
		t.Fatalf("barriers = %d, want one", len(barriers))
	}
	for _, commit := range barriers[0].Runs() {
		projection.runs[commit.Run.ID()] = *commit.Run
	}
	sessions.pending[spec.RunID] = barriers[0].Pending()
	owner, found := coordinator.registry.Get(spec.RunID)
	if !found {
		t.Fatal("maintenance lost its join owner")
	}
	if _, err := coordinator.Cancel(t.Context(), CancelCommand{RunID: spec.RunID}); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("cancel during maintenance = %v, want busy", err)
	}
	releaseFinish()
	if err := owner.owner.wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Cancel(t.Context(), CancelCommand{RunID: spec.RunID})
	if err != nil || result.Run.State() != run.Canceled {
		t.Fatalf("cancel after maintenance = %+v, %v", result, err)
	}
}
