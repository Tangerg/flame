package sessions

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// TestDeleteSessionAppliesThenReleasesExecutors: DeleteSession reads the open
// interrupts, commits the atomic delete write-set, then tears down the parked
// executions and the resume gate — in that order (the durable state is gone before the
// process-local cleanup).
func TestDeleteSessionAppliesThenReleasesExecutors(t *testing.T) {
	stores := newMutationStores("")
	executions := mutationExecutions{operations: &stores.operations}

	if err := newCoordinator(stores, executions).DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	want := []string{"interrupt.read", "session.quiesce", "apply.delete", "executor.release", "session.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
	if len(stores.deleted) != 1 || stores.deleted[0] != "ses_1" {
		t.Fatalf("deleted = %v, want [ses_1]", stores.deleted)
	}
}

// TestDeleteSessionStopsBeforeExecutorReleaseOnApplyFailure: a failed write-set
// leaves parked executions and the resume gate untouched (no half-cleanup on a
// durable failure).
func TestDeleteSessionStopsBeforeExecutorReleaseOnApplyFailure(t *testing.T) {
	stores := newMutationStores("apply.delete")
	executions := mutationExecutions{operations: &stores.operations}

	err := newCoordinator(stores, executions).DeleteSession(t.Context(), "ses_1")
	if !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if slices.Contains(stores.operations, "executor.release") || slices.Contains(stores.operations, "session.forget") {
		t.Fatalf("operations after failure = %v, want no executor release", stores.operations)
	}
}

func TestDeleteSessionStopsBeforeDurableMutationOnQuiesceFailure(t *testing.T) {
	stores := newMutationStores("session.quiesce")

	err := newCoordinator(stores, mutationExecutions{operations: &stores.operations}).DeleteSession(t.Context(), "ses_1")
	if !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	want := []string{"interrupt.read", "session.quiesce"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want no durable mutation after quiesce failure", stores.operations)
	}
	if len(stores.deleted) != 0 {
		t.Fatalf("deleted = %v, want none", stores.deleted)
	}
}

func TestDeleteSessionQuiescesGoalOnlyAfterDurableCommit(t *testing.T) {
	stores := newMutationStores("")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Goals:             mutationGoalGuard{operations: &stores.operations},
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	want := []string{"goal.mutation", "interrupt.read", "session.quiesce", "apply.delete", "goal.quiesce", "executor.release", "session.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestDeleteSessionDoesNotQuiesceGoalWhenDurableCommitFails(t *testing.T) {
	stores := newMutationStores("apply.delete")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Goals:             mutationGoalGuard{operations: &stores.operations},
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if slices.Contains(stores.operations, "goal.quiesce") {
		t.Fatalf("operations = %v, goal was quiesced after a failed write-set", stores.operations)
	}
}

// TestDeleteSessionCleansUpAfterGoalQuiesceFailure: the Session is gone as of
// the commit, so a Goal that would not quiesce cannot turn the delete into a
// failure — it settles, and the remaining cleanup still runs.
func TestDeleteSessionCleansUpAfterGoalQuiesceFailure(t *testing.T) {
	quiesceErr := errors.New("goal quiesce failed")
	stores := newMutationStores("")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Goals:             mutationGoalGuard{operations: &stores.operations, quiesceErr: quiesceErr},
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession reported a committed delete as failed: %v", err)
	}
	want := []string{"goal.mutation", "interrupt.read", "session.quiesce", "apply.delete", "goal.quiesce", "executor.release", "session.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want post-commit cleanup despite quiesce failure", stores.operations)
	}
	if len(stores.deleted) != 1 {
		t.Fatalf("deleted = %v, want the Session gone", stores.deleted)
	}
}

func TestDeleteSessionDetachesExecutorReleaseFromCallerCancellation(t *testing.T) {
	stores := newMutationStores("")
	executions := new(observingExecutions)
	ctx, cancel := context.WithCancel(t.Context())
	deps := testDependencies(stores, Dependencies{
		ExecutionReleaser: executions,
		Paths:             testWorkspaceResolver{},
	})
	// The caller gives up while the write-set is already committing: the durable
	// delete is past the point where abandoning it is an option, so the executor
	// teardown it owes must not inherit that cancellation.
	deps.TransientState = cancelingTransientState{stores: stores, cancel: cancel}

	if err := mustNewCoordinator(deps).DeleteSession(ctx, "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if executions.calls != 1 {
		t.Fatalf("execution Release calls = %d, want 1", executions.calls)
	}
	if executions.canceled {
		t.Fatal("execution release inherited caller cancellation")
	}
	if !executions.bounded {
		t.Fatal("execution release context has no deadline")
	}
}

// TestDeleteSessionKeepsTheSessionWhenItsOwnedStateCannotBeRetired: the Session
// is the only name for its checkpoint history and scratch tree, so a refusal to
// destroy either must leave the aggregate that still names them.
func TestDeleteSessionKeepsTheSessionWhenItsOwnedStateCannotBeRetired(t *testing.T) {
	for _, test := range []struct {
		name       string
		deps       func(stores *mutationStores) Dependencies
		wantErr    error
		operations []string
	}{
		{
			name:    "checkpoints",
			wantErr: errors.New("checkpoint cleanup failed"),
			operations: []string{
				"interrupt.read", "session.quiesce", "checkpoint.drop:ses_1",
			},
		},
		{
			name:    "sandbox",
			wantErr: errors.New("sandbox discard failed"),
			operations: []string{
				"interrupt.read", "session.quiesce", "checkpoint.drop:ses_1", "sandbox.discard:ses_1",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stores := newMutationStores("")
			checkpoints := &mutationCheckpoints{operations: &stores.operations}
			sandbox := &mutationSandbox{operations: &stores.operations}
			if test.name == "checkpoints" {
				checkpoints.err = test.wantErr
			} else {
				sandbox.err = test.wantErr
			}
			coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
				ExecutionReleaser: mutationExecutions{operations: &stores.operations},
				Paths:             testWorkspaceResolver{},
				Checkpoints:       checkpoints,
				Sandbox:           sandbox,
			}))

			err := coordinator.DeleteSession(t.Context(), "ses_1")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("DeleteSession error = %v, want %v", err, test.wantErr)
			}
			if !slices.Equal(stores.operations, test.operations) {
				t.Fatalf("operations = %v, want %v", stores.operations, test.operations)
			}
			if len(stores.deleted) != 0 {
				t.Fatalf("deleted = %v, want the Session kept beside the state it names", stores.deleted)
			}
		})
	}
}

// TestDeleteSessionCommitsDespiteExecutorReleaseFailure: an executor that will
// not release is process-local and outlives nothing, so it settles rather than
// telling the caller the Session is still there.
func TestDeleteSessionCommitsDespiteExecutorReleaseFailure(t *testing.T) {
	stores := newMutationStores("")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{
			operations: &stores.operations, err: errors.New("execution release failed"),
		},
		Paths:       testWorkspaceResolver{},
		Checkpoints: &mutationCheckpoints{operations: &stores.operations},
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession reported a committed delete as failed: %v", err)
	}
	want := []string{
		"interrupt.read", "session.quiesce", "checkpoint.drop:ses_1",
		"apply.delete", "executor.release", "session.forget",
	}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
	if len(stores.deleted) != 1 || stores.deleted[0] != "ses_1" {
		t.Fatal("cleanup failure prevented durable session deletion")
	}
}

// TestRollbackCommitsDespiteParkedExecutorReleaseFailure: the truncation is
// durable as of the commit, so an executor that refuses to release cannot make
// the rollback look unapplied — which used to strand its recovery intent and
// refuse every later Run on that Session and working tree.
func TestRollbackCommitsDespiteParkedExecutorReleaseFailure(t *testing.T) {
	stores := newMutationStores("")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{
			operations: &stores.operations, err: errors.New("execution release failed"),
		},
		Paths:   testWorkspaceResolver{},
		Sandbox: &mutationSandbox{operations: &stores.operations},
	}))
	boundary := transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_1"}}}

	if err := coordinator.applyRollback(t.Context(), "ses_1", boundary); err != nil {
		t.Fatalf("applyRollback reported a committed truncation as failed: %v", err)
	}
	want := []string{
		"session.quiesce",
		"sandbox.discard:ses_1",
		"apply.rollback",
		"session.context.forget",
		"executor.release",
	}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestRollbackDoesNotExposeHistoryWhenSandboxRetirementFails(t *testing.T) {
	wantErr := errors.New("sandbox cleanup failed")
	stores := newMutationStores("")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Sandbox:           &mutationSandbox{operations: &stores.operations, err: wantErr},
	}))
	boundary := transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_1"}}}

	err := coordinator.applyRollback(t.Context(), "ses_1", boundary)
	if !errors.Is(err, wantErr) {
		t.Fatalf("applyRollback error = %v, want sandbox cleanup failure", err)
	}
	want := []string{"session.quiesce", "sandbox.discard:ses_1"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want no durable rollback after sandbox failure", stores.operations)
	}
}

func TestDeleteSessionAddressesOnlyTheRequestedConversation(t *testing.T) {
	stores := newMutationStores("")
	claims := new(testClaimer)

	if err := newCoordinatorWithAdmissions(stores, mutationExecutions{operations: &stores.operations}, claims).DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	wantDeleted := []string{"ses_1"}
	if !slices.Equal(stores.deleted, wantDeleted) {
		t.Fatalf("deleted = %v, want %v", stores.deleted, wantDeleted)
	}
	if len(claims.claimed) != 0 || len(claims.released) != len(wantDeleted) {
		t.Fatalf("claims after delete = %+v releases=%v", claims.claimed, claims.released)
	}
}

// TestRestoreSessionAppliesPlan: RestoreSession forwards the decoded artifact to
// the atomic restore write-set verbatim.
func TestRestoreSessionAppliesPlan(t *testing.T) {
	stores := newMutationStores("")
	stores.pending = map[string][]runs.Pending{}
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Sandbox:           &mutationSandbox{operations: &stores.operations},
	}))
	_, err := coordinator.restoreSession(
		t.Context(),
		Snapshot{
			Session:  testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/workspace")}),
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hi"))},
		}, false,
	)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(stores.restored) != 1 || stores.restored[0].SessionReplacement().State().ID() != "ses_1" || len(stores.restored[0].Snapshot().Messages) != 1 {
		t.Fatalf("restored = %+v, want one plan for ses_1 with 1 message", stores.restored)
	}
	want := []string{"interrupt.read", "session.quiesce", "sandbox.discard:ses_1", "apply.restore", "session.context.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestRestoreSessionPresentsTheCommittedReplacementRevision(t *testing.T) {
	stores := newMutationStores("")
	stores.pending = map[string][]runs.Pending{}
	current := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_1", Workspace: testsupport.MustWorkspace("/workspace"), CreatedAt: time.Unix(1, 0).UTC(),
		UpdatedAt: time.Unix(1, 0).UTC(), Revision: 4,
	})
	stores.current = &current
	coordinator := newCoordinator(stores, mutationExecutions{operations: &stores.operations})

	view, err := coordinator.restoreSession(t.Context(), Snapshot{
		Session: testsupport.MustRestoreSession(session.Snapshot{
			ID: "ses_1", Workspace: testsupport.MustWorkspace("/workspace"), CreatedAt: time.Unix(1, 0).UTC(),
			UpdatedAt: time.Unix(1, 0).UTC(), Revision: 1,
		}),
	}, true)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if view.Revision != 5 {
		t.Fatalf("restored view revision = %d, want committed replacement revision 5", view.Revision)
	}
	if len(stores.restored) != 1 || stores.restored[0].SessionReplacement().State().Revision() != view.Revision {
		t.Fatalf("restored write/view revisions differ: plans=%+v view=%+v", stores.restored, view)
	}
}

func TestRestoreSessionRejectsUnresolvableCWDBeforeMutation(t *testing.T) {
	stores := newMutationStores("")
	stores.pending = map[string][]runs.Pending{}
	want := errors.New("missing workspace")
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{err: want},
	}))

	_, err := coordinator.restoreSession(t.Context(), Snapshot{Session: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/archive")})}, false)
	if !errors.Is(err, workspaceapp.ErrCWDUnavailable) || !errors.Is(err, want) {
		t.Fatalf("RestoreSession error = %v, want cwd unavailable + cause", err)
	}
	if len(stores.restored) != 0 {
		t.Fatalf("restore mutated storage after cwd rejection: %+v", stores.restored)
	}
}

func TestDeleteSessionRejectsInvalidIdentityBeforeLifecycleEffects(t *testing.T) {
	stores := newMutationStores("")
	coordinator := newCoordinator(stores, mutationExecutions{operations: &stores.operations})

	if err := coordinator.DeleteSession(t.Context(), "ses bad"); err == nil {
		t.Fatal("DeleteSession accepted an invalid Session identity")
	}
	if len(stores.operations) != 0 || len(stores.deleted) != 0 {
		t.Fatalf("invalid delete reached lifecycle effects: operations=%v deleted=%v", stores.operations, stores.deleted)
	}
}

var errMutationStage = errors.New("mutation stage failed")

type mutationGoalGuard struct {
	operations *[]string
	quiesceErr error
	settlement error
}

// WithSessionMutation mirrors the real guard's contract: a failed commit is the
// command's failure, and everything after it settles without changing it.
func (m mutationGoalGuard) WithSessionMutation(
	ctx context.Context,
	_ []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	*m.operations = append(*m.operations, "goal.mutation")
	if err := commit(ctx); err != nil {
		return err
	}
	*m.operations = append(*m.operations, "goal.quiesce")
	m.settlement = errors.Join(m.quiesceErr, afterCommit(ctx))
	return nil
}

// mutationStores supplies the coordinator's named persistence ports for mutation write-sets: it
// records the atomic Apply* calls + the executor release, and lists a single open
// interrupt so DeleteSession has a parked execution to release.
type mutationStores struct {
	operations []string
	fail       string
	deleted    []string
	restored   []RestorePlan
	ints       *mutationInterrupts
	pending    map[string][]runs.Pending
	current    *session.Session
}

func newMutationStores(fail string) *mutationStores {
	s := &mutationStores{
		fail: fail,
		pending: map[string][]runs.Pending{
			"ses_1": {testPending("run_1", "ses_1", time.Unix(1, 0).UTC())},
		},
	}
	s.ints = &mutationInterrupts{stores: s}
	return s
}

func (m *mutationStores) record(stage string) error {
	m.operations = append(m.operations, stage)
	if m.fail == stage {
		return errMutationStage
	}
	return nil
}

func (m *mutationStores) Session() Store                                       { return m }
func (m *mutationStores) Interrupts() InterruptStore                           { return m.ints }
func (m *mutationStores) Transcript() TranscriptStore                          { return emptyTranscript{} }
func (m *mutationStores) Runs() RunStore                                       { return emptyTranscript{} }
func (*mutationStores) ReadSnapshot(context.Context, string) (Snapshot, error) { panic("unused") }
func (m *mutationStores) ForgetSession(string) {
	m.operations = append(m.operations, "session.forget")
}
func (m *mutationStores) ForgetSessionContext(string) {
	m.operations = append(m.operations, "session.context.forget")
}
func (m *mutationStores) ForgetWorkspace(string) {
	m.operations = append(m.operations, "workspace.forget")
}
func (m *mutationStores) QuiesceSession(string) error {
	return m.record("session.quiesce")
}
func (m *mutationStores) QuiesceWorkspace(string) error {
	return m.record("workspace.quiesce")
}
func (*mutationStores) ApplyFork(context.Context, ForkPlan) (session.Session, error) {
	panic("unused")
}

func (m *mutationStores) ApplyRollback(context.Context, RollbackPlan) error {
	return m.record("apply.rollback")
}
func (m *mutationStores) ApplyRestore(_ context.Context, plan RestorePlan) error {
	if err := m.record("apply.restore"); err != nil {
		return err
	}
	m.restored = append(m.restored, plan)
	return nil
}
func (m *mutationStores) ApplyDelete(_ context.Context, plan DeletePlan) error {
	if err := m.record("apply.delete"); err != nil {
		return err
	}
	m.deleted = append(m.deleted, plan.SessionID())
	return nil
}
func (m *mutationStores) ApplyTerminal(context.Context, TerminalPlan) error {
	return m.record("apply.cancel")
}

func (*mutationStores) List(context.Context) ([]session.Session, error) { panic("unused") }

func (*mutationStores) ListPage(context.Context, session.CatalogRead) ([]session.Session, error) {
	panic("unused")
}
func (m *mutationStores) Get(context.Context, string) (session.Session, error) {
	if m.current != nil {
		return *m.current, nil
	}
	return session.Session{}, session.ErrNotFound
}
func (*mutationStores) Insert(context.Context, session.Session) error { panic("unused") }
func (*mutationStores) Save(context.Context, session.Replacement) error {
	panic("unused")
}

type mutationInterrupts struct{ stores *mutationStores }

func (m *mutationInterrupts) Open(context.Context, runs.Pending) error { panic("unused") }
func (m *mutationInterrupts) List(_ context.Context, sessionID string) ([]runs.Pending, error) {
	if err := m.stores.record("interrupt.read"); err != nil {
		return nil, err
	}
	return m.stores.pending[sessionID], nil
}
func (m *mutationInterrupts) Get(_ context.Context, runID string) (runs.Pending, bool, error) {
	for _, pending := range m.stores.pending {
		for _, item := range pending {
			if item.RootRunID == runID {
				return item, true, nil
			}
		}
	}
	return runs.Pending{}, false, nil
}
func (m *mutationInterrupts) Consume(context.Context, string, string) (runs.Pending, bool, error) {
	panic("unused")
}

type mutationExecutions struct {
	operations *[]string
	err        error
}

func (m mutationExecutions) Release(context.Context, runs.ExecutorRef) error {
	*m.operations = append(*m.operations, "executor.release")
	return m.err
}

type observingExecutions struct {
	calls    int
	canceled bool
	bounded  bool
}

func (o *observingExecutions) Release(ctx context.Context, _ runs.ExecutorRef) error {
	o.calls++
	o.canceled = ctx.Err() != nil
	_, o.bounded = ctx.Deadline()
	return nil
}

type mutationCheckpoints struct {
	operations *[]string
	err        error
}

func (*mutationCheckpoints) Restore(context.Context, string, string, string) error {
	panic("unused")
}

func (m *mutationCheckpoints) DropSession(sessionID string) error {
	*m.operations = append(*m.operations, "checkpoint.drop:"+sessionID)
	return m.err
}

type mutationSandbox struct {
	operations *[]string
	err        error
}

func (m *mutationSandbox) Discard(sessionID string) error {
	*m.operations = append(*m.operations, "sandbox.discard:"+sessionID)
	return m.err
}

// cancelingTransientState abandons the caller's request at the moment the
// write-set starts, so the settlement that follows a committed delete is the
// one under test.
type cancelingTransientState struct {
	stores *mutationStores
	cancel context.CancelFunc
}

func (c cancelingTransientState) QuiesceSession(sessionID string) error {
	err := c.stores.QuiesceSession(sessionID)
	c.cancel()
	return err
}

func (c cancelingTransientState) QuiesceWorkspace(root string) error {
	return c.stores.QuiesceWorkspace(root)
}
func (c cancelingTransientState) ForgetSession(sessionID string) { c.stores.ForgetSession(sessionID) }
func (c cancelingTransientState) ForgetSessionContext(sessionID string) {
	c.stores.ForgetSessionContext(sessionID)
}
func (c cancelingTransientState) ForgetWorkspace(root string) { c.stores.ForgetWorkspace(root) }
