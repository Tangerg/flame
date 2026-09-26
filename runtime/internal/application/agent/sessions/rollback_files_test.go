package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRollbackSpecOwnsClosedRestoreScope(t *testing.T) {
	valid := []struct {
		name    string
		scope   RestoreScope
		files   bool
		history bool
	}{
		{name: "history", scope: RestoreHistory, history: true},
		{name: "files", scope: RestoreFiles, files: true},
		{name: "both", scope: RestoreBoth, files: true, history: true},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			spec := RollbackSpec{SessionID: "ses_1", ToRunID: "run_1", Scope: test.scope}
			if err := spec.validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}
			if got := test.scope.RestoresFiles(); got != test.files {
				t.Fatalf("RestoresFiles = %v, want %v", got, test.files)
			}
			if got := test.scope.RestoresHistory(); got != test.history {
				t.Fatalf("RestoresHistory = %v, want %v", got, test.history)
			}
		})
	}
	if err := (RollbackSpec{SessionID: "ses_1", Scope: RestoreHistory}).validate(); err != nil {
		t.Fatalf("history rollback to an empty boundary: %v", err)
	}

	invalid := []RollbackSpec{
		{SessionID: "ses_1"},
		{SessionID: "ses_1", Scope: RestoreScope("workspace")},
		{SessionID: "ses_1", Scope: RestoreFiles},
		{SessionID: "ses_1", Scope: RestoreBoth},
	}
	for _, spec := range invalid {
		if err := spec.validate(); err == nil {
			t.Fatalf("validate(%+v) succeeded", spec)
		}
	}
}

func TestMutationCompletionDetachesFromCallerCancellation(t *testing.T) {
	mutations := new(observingMutations)
	coordinator := mustNewCoordinator(Dependencies{Mutations: mutations})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := coordinator.completeMutationDetached(ctx, WorkspaceMutation{SessionID: "ses_1"}); err != nil {
		t.Fatalf("completeMutationDetached: %v", err)
	}
	if mutations.canceled {
		t.Fatal("mutation cleanup inherited caller cancellation")
	}
	if !mutations.bounded {
		t.Fatal("mutation cleanup context has no deadline")
	}
}

type observingMutations struct {
	canceled    bool
	bounded     bool
	completeErr error
}

func (*observingMutations) Record(context.Context, WorkspaceMutation) (bool, error) { return true, nil }

func (o *observingMutations) Complete(ctx context.Context, _ WorkspaceMutation) error {
	o.canceled = ctx.Err() != nil
	_, o.bounded = ctx.Deadline()
	return o.completeErr
}

type refusedCheckpoint struct{ err error }

func (r refusedCheckpoint) Restore(context.Context, string, string, string) error { return r.err }
func (refusedCheckpoint) DropSession(string) error                                { return nil }

func TestRollbackRefusalKeepsCleanupFailureDistinctFromCompletedRefusal(t *testing.T) {
	for _, refusal := range []error{ErrCheckpointUnavailable, ErrCheckpointConflict} {
		t.Run(refusal.Error(), func(t *testing.T) {
			cleanupFailure := errors.New("intent cleanup failed")
			mutations := &observingMutations{completeErr: cleanupFailure}
			coordinator := mustNewCoordinator(Dependencies{Mutations: mutations, Checkpoints: refusedCheckpoint{err: refusal}})
			spec := RollbackSpec{SessionID: "ses_1", ToRunID: "run_1", Scope: RestoreBoth}
			mutation := WorkspaceMutation{SessionID: spec.SessionID, CWD: "/workspace", ToRunID: spec.ToRunID, RestoreHistory: true}
			err := coordinator.restoreRollbackFiles(t.Context(), spec, mutation.CWD, mutation, true)
			if !errors.Is(err, ErrRollbackRecoveryPending) || !errors.Is(err, cleanupFailure) || !errors.Is(err, refusal) {
				t.Fatalf("failed refusal cleanup = %v, want pending intent and both causes", err)
			}
			mutations.completeErr = nil
			err = coordinator.restoreRollbackFiles(t.Context(), spec, mutation.CWD, mutation, true)
			if !errors.Is(err, refusal) || errors.Is(err, ErrRollbackRecoveryPending) {
				t.Fatalf("completed refusal = %v, want definitive refusal", err)
			}
		})
	}
}

func (*observingMutations) ListPending(context.Context) ([]WorkspaceMutation, error) {
	return nil, nil
}

// TestRollbackReadsTheSessionItsClaimFroze: the claim is what stops a relocation
// from committing, so a Session read before it can name a working tree the
// Session no longer has — and answer with a workspace that moved.
func TestRollbackReadsTheSessionItsClaimFroze(t *testing.T) {
	stores := newMutationStores("")
	before := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_1", Workspace: testsupport.MustWorkspace("/before"),
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(), Revision: 1,
	})
	after := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_1", Workspace: testsupport.MustWorkspace("/after"),
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(), Revision: 2,
	})
	stores.current = &before
	deps := testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
	})
	// The relocation commits in the instant before this rollback owns the Session.
	deps.Admissions = &relocatingClaimer{claimer: new(testClaimer), stores: stores, after: &after}

	result, err := mustNewCoordinator(deps).Rollback(t.Context(), RollbackSpec{
		SessionID: "ses_1", Scope: RestoreHistory,
	})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if result.Session.Workspace.Path != "/after" || result.Session.Revision != 2 {
		t.Fatalf("rollback Session = %+v, want the relocated workspace at revision 2", result.Session)
	}
}

// relocatingClaimer commits a Session relocation at the moment the rollback
// takes the claim that would have refused it.
type relocatingClaimer struct {
	claimer *testClaimer
	stores  *mutationStores
	after   *session.Session
}

func (r *relocatingClaimer) AcquireSession(ctx context.Context, sessionID string) (func(), bool, error) {
	release, ok, err := r.claimer.AcquireSession(ctx, sessionID)
	if ok {
		r.stores.current = r.after
	}
	return release, ok, err
}

func (r *relocatingClaimer) AcquireWorkingTreeMutation(cwd string) (func(), bool, error) {
	return r.claimer.AcquireWorkingTreeMutation(cwd)
}
