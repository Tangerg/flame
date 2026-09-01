package session

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

type recordingRuntime struct {
	*runtimefixture.Runtime

	calls     int
	request   agent.RollbackSession
	reject    error
	afterCall func()
}

func protectedRollbackGuard(t *testing.T, namespace string, until time.Time) commandreplay.Guard {
	t.Helper()
	guard, err := commandreplay.NewProtectedGuard(namespace, until)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func advertisedRollbackPolicy(
	t *testing.T,
	namespace string,
	retention time.Duration,
	now func() time.Time,
) commandreplay.Policy {
	t.Helper()
	capability, err := commandreplay.NewCapability(namespace, retention)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := commandreplay.NewPolicyWithClock(capability, now)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func unavailableRollbackPolicy(t *testing.T, now func() time.Time) commandreplay.Policy {
	t.Helper()
	policy, err := commandreplay.UnavailablePolicyWithClock(now)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func (r *recordingRuntime) RollbackSession(
	ctx context.Context,
	request agent.RollbackSession,
) (agent.RollbackResult, error) {
	r.calls++
	r.request = request
	reject := r.reject
	if r.afterCall != nil {
		r.afterCall()
	}
	if reject != nil {
		return agent.RollbackResult{}, reject
	}
	return r.Runtime.RollbackSession(ctx, request)
}

func TestFileRollbackStopsRetryingWhenReplayExpires(t *testing.T) {
	underlying := runtimefixture.New()
	snapshot, err := underlying.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewRollback(snapshot, agent.RollbackSession{
		SessionID: snapshot.Session.ID, ToRunID: snapshot.Runs[0].ID, Scope: agent.RestoreFiles,
	})
	if err != nil {
		t.Fatal(err)
	}
	stagedAt := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	replay := protectedRollbackGuard(t, "idp_original", stagedAt.Add(time.Minute))
	pending := preview.journal(
		agent.CommandID("cli_99999999999999999999999999999999"), replay, stagedAt,
	)
	now := pending.Replay.Until().Add(-time.Nanosecond)
	policy := advertisedRollbackPolicy(t, "idp_original", time.Minute, func() time.Time { return now })
	runtime := &recordingRuntime{Runtime: underlying, reject: agent.ErrDisconnected}
	runtime.afterCall = func() {
		now = pending.Replay.Until()
		runtime.reject = nil
	}
	result, err := settleRollback(t.Context(), runtime, pending, policy, retry.ImmediateBackoff(), false)
	if result.Outcome != mutation.Unknown || !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("settlement = outcome %v, error %v", result.Outcome, err)
	}
	if runtime.calls != 1 {
		t.Fatalf("expired rollback reached runtime %d times", runtime.calls)
	}
}

func rollbackFixture(t *testing.T, request agent.RollbackSession) (*runtimefixture.Runtime, RollbackPreview) {
	t.Helper()
	runtime := runtimefixture.New()
	snapshot, err := runtime.GetSession(t.Context(), request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewRollback(snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, preview
}

func TestRecoverConfirmsAnAlreadyAppliedRollbackWithoutReplay(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	policy := unavailableRollbackPolicy(t, func() time.Time { return now })
	pending := preview.journal(
		agent.CommandID("cli_11111111111111111111111111111111"), commandreplay.UnprotectedGuard(), now,
	)
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if stageSessionRollbackErr := store.StageSessionRollback(pending); stageSessionRollbackErr != nil {
		t.Fatal(stageSessionRollbackErr)
	}
	if _, rollbackSessionErr := underlying.RollbackSession(t.Context(), pending.Request()); rollbackSessionErr != nil {
		t.Fatal(rollbackSessionErr)
	}
	runtime := &recordingRuntime{Runtime: underlying}
	if recoverErr := RecoverRollbacks(t.Context(), runtime, store, policy, retry.ImmediateBackoff()); recoverErr != nil {
		t.Fatal(recoverErr)
	}
	if runtime.calls != 0 {
		t.Fatalf("already-applied rollback was replayed %d times", runtime.calls)
	}
	confirmed, exists := store.PendingSessionRollback(pending.SessionID)
	if !exists || confirmed.Phase != workbench.SessionRollbackConfirmed {
		t.Fatalf("confirmed rollback = %+v, present %t", confirmed, exists)
	}
	recovery, found, err := store.ConsumeConfirmedSessionRollback(pending.SessionID)
	if err != nil || !found || recovery.Draft.Text != "Why is the cache expiry test flaky?" {
		t.Fatalf("rollback recovery = %+v, present %t, err %v", recovery, found, err)
	}
}

func TestPreviewKeepsTheBoundaryRootDescendants(t *testing.T) {
	runtime := runtimefixture.New()
	snapshot, err := runtime.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	root := snapshot.Runs[0]
	child := root.Clone()
	child.ID = "run_child"
	child.Lineage, err = agent.NewChildRunLineage(child.ID, "item_delegate", root.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	child.CreatedAt = root.CreatedAt.Add(time.Millisecond)
	later := root.Clone()
	later.ID = "run_later"
	later.CreatedAt = root.CreatedAt.Add(2 * time.Millisecond)
	snapshot.Runs = []agent.Run{root, child, later}
	if validateErr := snapshot.Validate(); validateErr != nil {
		t.Fatal(validateErr)
	}
	preview, err := PreviewRollback(snapshot, agent.RollbackSession{
		SessionID: snapshot.Session.ID, ToRunID: root.ID, Scope: agent.RestoreHistory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(preview.afterRunIDs, []string{root.ID, child.ID}) || preview.DroppedCount() != 1 {
		t.Fatalf("rollback projection after = %v, dropped %d", preview.afterRunIDs, preview.DroppedCount())
	}
	applied := snapshot
	applied.Session.Revision++
	applied.Runs = slices.Clone(snapshot.Runs[:2])
	if err := preview.ValidateApplied(applied); err != nil {
		t.Fatalf("root subtree rollback outcome: %v", err)
	}
}

func TestRecoverReplaysAPreparedHistoryRollbackWithItsStableIdentity(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	policy := unavailableRollbackPolicy(t, func() time.Time { return now })
	pending := preview.journal(
		agent.CommandID("cli_22222222222222222222222222222222"), commandreplay.UnprotectedGuard(), now,
	)
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StageSessionRollback(pending); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{Runtime: underlying}
	if err := RecoverRollbacks(t.Context(), runtime, store, policy, retry.ImmediateBackoff()); err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 1 || runtime.request != pending.Request() {
		t.Fatalf("rollback replay = %+v after %d calls", runtime.request, runtime.calls)
	}
	confirmed, exists := store.PendingSessionRollback(pending.SessionID)
	if !exists || confirmed.Phase != workbench.SessionRollbackConfirmed {
		t.Fatalf("confirmed rollback = %+v, present %t", confirmed, exists)
	}
}

func TestRecoverRefusesUnprovenFileRollbackReplay(t *testing.T) {
	for _, test := range []struct {
		name      string
		namespace string
		now       time.Time
	}{
		{
			name:      "another runtime namespace",
			namespace: "idp_other", now: time.Date(2026, 8, 13, 10, 0, 30, 0, time.UTC),
		},
		{
			name:      "replay deadline reached",
			namespace: "idp_original", now: time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC),
		},
		{
			name:      "expired replay window",
			namespace: "idp_original", now: time.Date(2026, 8, 13, 10, 2, 0, 0, time.UTC),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			underlying := runtimefixture.New()
			snapshot, err := underlying.GetSession(t.Context(), "ses_demo_1")
			if err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewRollback(snapshot, agent.RollbackSession{
				SessionID: snapshot.Session.ID, ToRunID: snapshot.Runs[0].ID, Scope: agent.RestoreFiles,
			})
			if err != nil {
				t.Fatal(err)
			}
			stagedAt := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
			pending := preview.journal(
				agent.CommandID("cli_33333333333333333333333333333333"),
				protectedRollbackGuard(t, "idp_original", stagedAt.Add(time.Minute)), stagedAt,
			)
			store, err := openTestWorkbench(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if stageSessionRollbackErr := store.StageSessionRollback(pending); stageSessionRollbackErr != nil {
				t.Fatal(stageSessionRollbackErr)
			}
			runtime := &recordingRuntime{Runtime: underlying}
			policy := advertisedRollbackPolicy(t, test.namespace, time.Minute, func() time.Time { return test.now })
			err = RecoverRollbacks(t.Context(), runtime, store, policy, retry.ImmediateBackoff())
			if err == nil || !strings.Contains(err.Error(), "replay guarantee") {
				t.Fatalf("file rollback recovery error = %v", err)
			}
			if runtime.calls != 0 {
				t.Fatalf("unsafe file rollback was replayed %d times", runtime.calls)
			}
			stored, exists := store.PendingSessionRollback(pending.SessionID)
			if !exists || stored.Phase != workbench.SessionRollbackPrepared {
				t.Fatalf("preserved file rollback = %+v, present %t", stored, exists)
			}
		})
	}
}

func TestRecoverRetiresADefinitivelyRejectedHistoryRollback(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	policy := unavailableRollbackPolicy(t, func() time.Time { return now })
	pending := preview.journal(
		agent.CommandID("cli_44444444444444444444444444444444"), commandreplay.UnprotectedGuard(), now,
	)
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StageSessionRollback(pending); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{Runtime: underlying, reject: agent.ErrSessionBusy}
	if err := RecoverRollbacks(t.Context(), runtime, store, policy, retry.ImmediateBackoff()); err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 1 || !errors.Is(runtime.reject, agent.ErrSessionBusy) {
		t.Fatalf("rejected rollback calls = %d, error %v", runtime.calls, runtime.reject)
	}
	if pending := store.PendingSessionRollbacks(); len(pending) != 0 {
		t.Fatalf("rejected rollback journals = %+v", pending)
	}
}

func TestRecoverPreservesHistoryRollbackRejectedByAnotherRuntimeStore(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	policy := unavailableRollbackPolicy(t, func() time.Time { return now })
	pending := preview.journal(
		agent.CommandID("cli_55555555555555555555555555555555"), commandreplay.UnprotectedGuard(), now,
	)
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if stageSessionRollbackErr := store.StageSessionRollback(pending); stageSessionRollbackErr != nil {
		t.Fatal(stageSessionRollbackErr)
	}
	runtime := &recordingRuntime{Runtime: underlying, reject: agent.ErrCommandStoreMismatch}
	err = RecoverRollbacks(t.Context(), runtime, store, policy, retry.ImmediateBackoff())
	if !errors.Is(err, agent.ErrCommandStoreMismatch) {
		t.Fatalf("store mismatch recovery error = %v", err)
	}
	stored, exists := store.PendingSessionRollback(pending.SessionID)
	if !exists || stored.CommandID != pending.CommandID || stored.Phase != workbench.SessionRollbackPrepared {
		t.Fatalf("preserved rollback = %+v, present %t", stored, exists)
	}
}
