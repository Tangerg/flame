package runs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type retainedContext struct{}

func (retainedContext) LoadSessionCheckpoint(context.Context, string) (ExecutorCheckpoint, bool, error) {
	return ExecutorCheckpoint{}, true, nil
}

type connectingSource struct {
	uncertainExecutionRecovery
	verdict RecoveryVerdict
	probes  int
}

func (s *connectingSource) PrepareUncertainExecution(context.Context, ActiveExecution) (UncertainExecutionPlan, RecoveryVerdict, error) {
	s.probes++
	return UncertainExecutionPlan{}, s.verdict, nil
}

func TestUncertainRevivalFencesOnlyItsPendingSession(t *testing.T) {
	at := time.Unix(1, 0)
	pending := testApprovalPending("member_1", at)
	sessions := &fakeRunSessions{sess: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}), pending: map[string]Pending{"run_1": pending}}
	c := newUseCaseCoordinator(&fakeExecutor{}, &fakeExecutionPorts{}, sessions, &fakeEffects{}).Coordinator
	parked := runForPending(pending)
	root, err := parked.Replace().Resume("seg_r", parked.UpdatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	c.runs.(*fakeRunProjection).runs["run_1"] = root.State()
	store := &recoveryStoreStub{runs: []run.Run{root.State()}}
	source := &connectingSource{verdict: RecoveryDeferred}
	var revival UncertainRevival
	if err := revival.Reconcile(t.Context(), c, store, retainedContext{}, source); !errors.Is(err, ErrOriginalSourcePending) {
		t.Fatalf("pending: %v", err)
	}
	if release, ok, err := c.admission.AcquireRecoverySession("ses_1"); err != nil || ok {
		if ok {
			release()
		}
		t.Fatal("pending session lost its fence", err)
	}
	release, ok, err := c.admission.AcquireRecoverySession("ses_other")
	if err != nil || !ok {
		t.Fatal("unrelated session blocked", err)
	}
	release()
	source.verdict = RecoveryUnavailable
	if err := revival.Reconcile(t.Context(), c, store, retainedContext{}, source); err != nil || source.probes != 2 {
		t.Fatal("original source was not reevaluated", err)
	}
	release, ok, err = c.admission.AcquireRecoverySession("ses_1")
	if err != nil || !ok {
		t.Fatal("conclusive result kept fence", err)
	}
	release()
}
