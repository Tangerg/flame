package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

type deletionRuntimeStub struct {
	deleteErr   error
	readErr     error
	deletes     int
	reads       int
	afterDelete func()
}

func (d *deletionRuntimeStub) DeleteSession(context.Context, conversation.DeleteSession) error {
	d.deletes++
	err := d.deleteErr
	if d.afterDelete != nil {
		d.afterDelete()
	}
	return err
}

func TestRecoverDoesNotReplayADeletionIntoAnotherRuntimeStore(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := conversation.DeleteSession{
		CommandID: "cli_77777777777777777777777777777777", SessionID: "ses_1",
	}
	if stageSessionDeletionErr := store.StageSessionDeletion(
		request, protectedGuard(t, "runtime-a", time.Now().UTC().Add(time.Hour)),
	); stageSessionDeletionErr != nil {
		t.Fatal(stageSessionDeletionErr)
	}
	runtime := new(deletionRuntimeStub)
	err = RecoverDeletions(
		t.Context(), runtime, store,
		replayPolicy(t, "runtime-b", time.Hour, time.Now), fastBackoff(t),
	)
	if err == nil {
		t.Fatal("cross-store deletion recovery unexpectedly succeeded")
	}
	if runtime.deletes != 0 || runtime.reads != 0 {
		t.Fatalf("cross-store recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); !found || pending.CommandID != request.CommandID {
		t.Fatalf("preserved deletion = %+v, found %t", pending, found)
	}
}

func TestDeletionReplayGuaranteeExpiresAtItsDeadline(t *testing.T) {
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	guard := protectedGuard(t, "runtime-a", deadline)
	policy := replayPolicy(t, "runtime-a", time.Minute, func() time.Time { return deadline })
	if policy.Replayable(guard) {
		t.Fatal("deletion replay remained safe at its retention deadline")
	}
}

func (d *deletionRuntimeStub) GetSession(context.Context, string) (conversation.SessionSnapshot, error) {
	d.reads++
	return conversation.SessionSnapshot{}, d.readErr
}

func TestRecoverRetiresAnExpiredDeletionProvenByTheOwningRuntime(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := conversation.DeleteSession{
		CommandID: "cli_99999999999999999999999999999999", SessionID: "ses_1",
	}
	deadline := time.Now().UTC().Add(-time.Second)
	if stageSessionDeletionErr := store.StageSessionDeletion(request, protectedGuard(t, "runtime-a", deadline)); stageSessionDeletionErr != nil {
		t.Fatal(stageSessionDeletionErr)
	}
	runtime := &deletionRuntimeStub{readErr: conversation.ErrSessionNotFound}
	err = RecoverDeletions(
		t.Context(), runtime, store,
		replayPolicy(t, "runtime-a", time.Hour, time.Now), fastBackoff(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.deletes != 0 || runtime.reads != 1 {
		t.Fatalf("settled recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); found {
		t.Fatalf("settled deletion remains durable: %+v", pending)
	}
}

func TestExecuteConfirmsAnExpiredDeletionProvenByTheOwningRuntime(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := conversation.DeleteSession{
		CommandID: "cli_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SessionID: "ses_1",
	}
	if stageSessionDeletionErr := store.StageSessionDeletion(
		request, protectedGuard(t, "runtime-a", time.Now().UTC().Add(-time.Second)),
	); stageSessionDeletionErr != nil {
		t.Fatal(stageSessionDeletionErr)
	}
	runtime := &deletionRuntimeStub{readErr: conversation.ErrSessionNotFound}
	result, err := Delete(
		t.Context(), runtime, store, request.SessionID,
		replayPolicy(t, "runtime-a", time.Hour, time.Now), fastBackoff(t),
	)
	if err != nil || result.Outcome != mutation.Confirmed || result.Request != request {
		t.Fatalf("settlement = %+v, %v", result, err)
	}
	if runtime.deletes != 0 || runtime.reads != 1 {
		t.Fatalf("settled execution performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
}

func TestExecuteRejectsAnExpiredDeletionWhenTheSessionStillExists(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := conversation.DeleteSession{
		CommandID: "cli_abababababababababababababababab", SessionID: "ses_1",
	}
	if stageSessionDeletionErr := store.StageSessionDeletion(
		request, protectedGuard(t, "runtime-a", time.Now().UTC().Add(-time.Second)),
	); stageSessionDeletionErr != nil {
		t.Fatal(stageSessionDeletionErr)
	}
	runtime := new(deletionRuntimeStub)
	result, err := Delete(
		t.Context(), runtime, store, request.SessionID,
		replayPolicy(t, "runtime-a", time.Hour, time.Now), fastBackoff(t),
	)
	if err != nil || result.Outcome != mutation.Rejected || result.Request != request {
		t.Fatalf("settlement = %+v, %v", result, err)
	}
	if runtime.deletes != 0 || runtime.reads != 1 {
		t.Fatalf("rejected execution performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
}

func TestSettlePreservesDeletionRejectedByAnotherRuntimeStore(t *testing.T) {
	runtime := &deletionRuntimeStub{deleteErr: conversation.ErrCommandStoreMismatch}
	request := conversation.DeleteSession{
		CommandID: "cli_66666666666666666666666666666666", SessionID: "ses_1",
	}
	deadline := time.Now().UTC().Add(time.Hour)
	policy := replayPolicy(t, "runtime-a", time.Hour, time.Now)
	outcome, err := settleDeletion(
		t.Context(), runtime, request, protectedGuard(t, "runtime-a", deadline),
		policy, fastBackoff(t),
	)
	if outcome != mutation.Unknown || !errors.Is(err, conversation.ErrCommandStoreMismatch) {
		t.Fatalf("store mismatch settlement = outcome %v, error %v", outcome, err)
	}
	if runtime.reads != 0 {
		t.Fatalf("store mismatch consulted %d projections from the wrong store", runtime.reads)
	}
}

func TestRecoverRejectsAnUncommittedDeletionWhenReplayExpires(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	request := conversation.DeleteSession{
		CommandID: "cli_88888888888888888888888888888888", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, protectedGuard(t, "runtime-a", deadline)); err != nil {
		t.Fatal(err)
	}
	now := deadline.Add(-time.Nanosecond)
	policy := replayPolicy(t, "runtime-a", time.Minute, func() time.Time { return now })
	runtime := &deletionRuntimeStub{deleteErr: conversation.ErrDisconnected}
	runtime.afterDelete = func() {
		now = deadline
		runtime.deleteErr = nil
	}
	if err := RecoverDeletions(t.Context(), runtime, store, policy, fastBackoff(t)); err != nil {
		t.Fatal(err)
	}
	if runtime.deletes != 1 || runtime.reads != 1 {
		t.Fatalf("expired recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); found {
		t.Fatalf("rejected deletion remains durable: %+v", pending)
	}
}

func TestRecoverConvergesADeletionCommittedAsReplayExpires(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	request := conversation.DeleteSession{
		CommandID: "cli_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, protectedGuard(t, "runtime-a", deadline)); err != nil {
		t.Fatal(err)
	}
	now := deadline.Add(-time.Nanosecond)
	policy := replayPolicy(t, "runtime-a", time.Minute, func() time.Time { return now })
	runtime := &deletionRuntimeStub{deleteErr: conversation.ErrDisconnected}
	runtime.afterDelete = func() {
		now = deadline
		runtime.readErr = conversation.ErrSessionNotFound
	}
	if err := RecoverDeletions(t.Context(), runtime, store, policy, fastBackoff(t)); err != nil {
		t.Fatal(err)
	}
	if runtime.deletes != 1 || runtime.reads != 1 {
		t.Fatalf("converged recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); found {
		t.Fatalf("converged deletion remains durable: %+v", pending)
	}
}

func protectedGuard(t *testing.T, namespace string, until time.Time) replay.Guard {
	t.Helper()
	guard, err := replay.NewProtectedGuard(namespace, until)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func replayPolicy(
	t *testing.T,
	namespace string,
	retention time.Duration,
	now func() time.Time,
) mutation.ReplayPolicy {
	t.Helper()
	capability, err := replay.NewCapability(namespace, retention)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := mutation.NewReplayPolicy(capability, now)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

// fastBackoff is an ordinary bounded schedule whose floor is short enough that
// a retry loop finishes within a test. Production configures the same shape.
func fastBackoff(t testing.TB) retry.Backoff {
	t.Helper()
	backoff, err := retry.NewBackoff(time.Nanosecond, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	return backoff
}
