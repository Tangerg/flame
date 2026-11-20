package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

const lostToolResult = "tool result unavailable because execution state was lost"

// RecoveryStore exposes durable application facts and atomically applies the
// recovery plan derived from them. It never validates executor payloads or
// decides which Run tree survives.
type RecoveryStore interface {
	ToolResultPublications
	// ListNonTerminalRuns returns the complete, unique set of constructed open
	// Runs, with at most one root per Session. Storage owns this query contract.
	ListNonTerminalRuns(ctx context.Context) ([]rundomain.Run, error)
	ListPendingInterrupts(ctx context.Context) ([]Pending, error)
	ListOpenModelInvocations(ctx context.Context) ([]OpenModelInvocation, error)
	ListIndependentEndpoints(context.Context) ([]modelref.EndpointObservation, error)
	ListOpenToolInvocations(ctx context.Context) ([]OpenToolInvocation, error)
	SessionByID(ctx context.Context, sessionID identity.SessionID) (session.Session, error)
	ListTranscript(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error)
	LoadExecutorCheckpoint(ctx context.Context, rootMemberID string) (ExecutorCheckpoint, error)
	ReadMessages(ctx context.Context, sessionID identity.SessionID) ([]corechat.Message, error)
	CommitRecovery(ctx context.Context, commit RecoveryCommit) error
}

// WaitingExecutionResumability is the recovery use case's narrow executor
// probe. The Application supplies the exact durable continuation; an
// unresumable verdict names why its opaque state cannot continue, while an
// error means the probe itself was inconclusive and startup must stop without
// writes.
type WaitingExecutionResumability interface {
	CanResumeWaitingExecution(ctx context.Context, continuation ExecutionContinuation) (WaitingResumption, error)
}

// recoveryAdmissions provides the same per-Session writer boundary used by
// live Run and Session mutations. Recovery skips a Session when another
// Runtime process still owns it; such a Run is live, not abandoned.
type recoveryAdmissions interface {
	AcquireRecoverySession(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error)
}

// RecoveryCommit is the complete atomic write-set for one ownership-scoped
// reconciliation. LostRuns retain the exact non-terminal aggregate and its
// child-before-parent replacement. Checkpoint and callback cleanup name only
// Sessions whose writer lease was acquired.
type RecoveryCommit struct {
	state RecoveryCommitInput
}

// RecoveryCommitInput is the unvalidated material [NewRecoveryCommit] freezes.
// It is a value rather than a parameter list because its two Session-identity
// members carry opposite checkpoint authority and must not be transposable.
type RecoveryCommitInput struct {
	LostRuns                []rundomain.Replacement
	ItemReplacements        []transcript.Replacement
	ConversationTransitions []RecoveryConversationTransition
	ModelInvocations        []ModelInvocationRecovery
	IndependentEndpoints    []modelref.EndpointObservation
	ToolInvocations         []ToolInvocationRecovery
	// DeleteInterrupts names every lost root's interrupt record, including one
	// hidden in the resuming state after an answer claim; deletion is idempotent.
	DeleteInterrupts []identity.RunID
	// PreservedSessionIDs names waiting trees whose compatible checkpoint keeps
	// them live. Together with lost-tree Sessions it owns the exact callback
	// cleanup scope.
	PreservedSessionIDs []identity.SessionID
	// DeleteCheckpointSessionIDs names recovered lost trees. A preserved waiting
	// tree is intentionally absent so its opaque checkpoint remains available.
	DeleteCheckpointSessionIDs []identity.SessionID
}

// RecoveryConversationTransition closes the model context for one lost Run
// tree. ExpectedCount is the boot snapshot's durable watermark; Messages is
// empty when the context was already closed, or one Tool message containing an
// error result for every unresolved provider ToolCall.
type RecoveryConversationTransition struct {
	RootRunID     identity.RunID
	SessionID     identity.SessionID
	ExpectedCount int
	Messages      []corechat.Message
}

// Recovery owns the application policy that reconciles Run trees abandoned by
// a dead Runtime process. Each pass claims Session writer ownership before
// re-reading candidates; CommitRecovery applies the resulting write-set
// atomically and Run transitions remain CAS guarded by storage.
type Recovery struct {
	store                RecoveryStore
	subtreeCancellations *Cancellations
	resumability         WaitingExecutionResumability
	admissions           recoveryAdmissions
	invalidations        invalidation.Publish
	now                  func() time.Time
}

// NewRecovery constructs the Run ownership recovery use case.
func NewRecovery(
	store RecoveryStore,
	subtreeCancellations *Cancellations,
	resumability WaitingExecutionResumability,
	admissions recoveryAdmissions,
	invalidations invalidation.Publish,
) (*Recovery, error) {
	if lo.IsNil(store) {
		return nil, errors.New("runs: recovery store is required")
	}
	if subtreeCancellations == nil {
		return nil, errors.New("runs: accepted subtree cancellations are required")
	}
	if lo.IsNil(resumability) {
		return nil, errors.New("runs: waiting execution resumability is required")
	}
	if lo.IsNil(admissions) {
		return nil, errors.New("runs: recovery admissions are required")
	}
	if invalidations == nil {
		return nil, errors.New("runs: recovery invalidations are required")
	}
	return &Recovery{
		store:                store,
		subtreeCancellations: subtreeCancellations,
		resumability:         resumability,
		admissions:           admissions,
		invalidations:        invalidations,
		now:                  time.Now,
	}, nil
}

// Reconcile preserves only complete waiting trees whose durable hand-off
// and opaque executor checkpoint remain coherent. Every other non-terminal tree
// is recovered as run_lost in one application transaction.
func (r *Recovery) Reconcile(ctx context.Context) (int, error) {
	if err := r.subtreeCancellations.ReviveAcceptedSubtreeCancellations(ctx); err != nil {
		return 0, err
	}
	claims, err := r.claimAbandonedSessions(ctx)
	if err != nil {
		return 0, err
	}
	defer claims.release()
	planner, err := newRecoveryPlanner(ctx, r, claims)
	if err != nil {
		return 0, err
	}
	commit, reconciled, err := planner.plan()
	if err != nil {
		return 0, err
	}
	if err := r.store.CommitRecovery(ctx, commit); err != nil {
		return 0, fmt.Errorf("runs: commit ownership recovery: %w", err)
	}
	r.publishRecoveredReadModels(commit)
	return reconciled, nil
}

// recoverySessionClaims owns the Session writer claims for one reconciliation
// pass. The claimed snapshot and release stack cannot be separated, and release
// is exact-once across every planning or commit failure.
type recoverySessionClaims struct {
	active     []rundomain.Run
	sessionIDs map[identity.SessionID]struct{}
	releases   []func()
	released   bool
}

func (r *recoverySessionClaims) includes(sessionID identity.SessionID) bool {
	_, ok := r.sessionIDs[sessionID]
	return ok
}

// activeRoots names the tree root of every claimed active Run with its Session.
func (r *recoverySessionClaims) activeRoots() map[identity.RunID]identity.SessionID {
	roots := make(map[identity.RunID]identity.SessionID, len(r.active))
	for _, value := range r.active {
		roots[value.Lineage().TreeRootID(value.ID())] = value.SessionID()
	}
	return roots
}

func (r *recoverySessionClaims) release() {
	if r == nil || r.released {
		return
	}
	r.released = true
	for index := len(r.releases) - 1; index >= 0; index-- {
		r.releases[index]()
	}
}

// publishRecoveredReadModels reports only the durable fact scopes this recovery
// write-set can change. Periodic survivor recovery commits through this Runtime's
// own SQLite connection, so PRAGMA data_version deliberately cannot observe it;
// without an application notice, already-mounted clients would remain pinned to
// the dead owner's Run/HITL projection even after the database reached RunLost.
func (r *Recovery) publishRecoveredReadModels(commit RecoveryCommit) {
	type sessionChanges struct {
		runIDs  []identity.RunID
		rootIDs []identity.RunID
		goal    bool
	}
	changes := make(map[identity.SessionID]*sessionChanges)
	for _, recovery := range commit.LostRuns() {
		lost := recovery.State()
		scope := changes[lost.SessionID()]
		if scope == nil {
			scope = &sessionChanges{}
			changes[lost.SessionID()] = scope
		}
		scope.runIDs = append(scope.runIDs, lost.ID())
	}
	if len(changes) == 0 {
		return
	}
	sessionOfRun := make(map[identity.RunID]identity.SessionID)
	for _, recovery := range commit.LostRuns() {
		sessionOfRun[recovery.State().ID()] = recovery.State().SessionID()
	}
	for _, rootRunID := range commit.DeleteInterrupts() {
		if scope := changes[sessionOfRun[rootRunID]]; scope != nil {
			scope.rootIDs = append(scope.rootIDs, rootRunID)
		}
	}
	for _, charged := range commit.GoalRuns() {
		if scope := changes[charged.SessionID()]; scope != nil {
			scope.goal = true
		}
	}
	sessionIDs := make([]identity.SessionID, 0, len(changes))
	for sessionID := range changes {
		sessionIDs = append(sessionIDs, sessionID)
	}
	slices.SortFunc(sessionIDs, identity.SessionID.Compare)
	for _, sessionID := range sessionIDs {
		scope := changes[sessionID]
		slices.SortFunc(scope.runIDs, identity.RunID.Compare)
		scope.runIDs = slices.Compact(scope.runIDs)
		slices.SortFunc(scope.rootIDs, identity.RunID.Compare)
		scope.rootIDs = slices.Compact(scope.rootIDs)
		notices := []invalidation.Notice{
			invalidation.InSession(invalidation.Runs, sessionID, scope.runIDs...),
		}
		if len(scope.rootIDs) > 0 {
			notices = append(notices,
				invalidation.InSession(invalidation.Interrupts, sessionID, scope.rootIDs...),
			)
		}
		notices = append(notices, invalidation.InSession(invalidation.Sessions, sessionID))
		if scope.goal {
			notices = append(notices, invalidation.InSession(invalidation.Goals, sessionID))
		}
		r.invalidations.Notify(notices...)
	}
}

func (r *Recovery) claimAbandonedSessions(
	ctx context.Context,
) (*recoverySessionClaims, error) {
	candidates, err := r.store.ListNonTerminalRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("runs: load recovery candidates: %w", err)
	}
	ids := recoveryRunCatalogSessionIDs(candidates)
	claims := &recoverySessionClaims{
		sessionIDs: make(map[identity.SessionID]struct{}, len(ids)),
		releases:   make([]func(), 0, len(ids)),
	}
	for _, sessionID := range ids {
		release, ok, err := r.admissions.AcquireRecoverySession(ctx, sessionID)
		if err != nil {
			claims.release()
			return nil, fmt.Errorf("runs: acquire recovery Session %q: %w", sessionID, err)
		}
		if ok {
			claims.sessionIDs[sessionID] = struct{}{}
			claims.releases = append(claims.releases, release)
		}
	}
	protected, err := r.subtreeCancellations.unrevivedSubtreeCancellations(ctx)
	if err != nil {
		claims.release()
		return nil, fmt.Errorf("runs: read accepted subtree cancellations under recovery admission: %w", err)
	}
	for _, value := range protected {
		delete(claims.sessionIDs, value.SessionID)
	}
	current, err := r.store.ListNonTerminalRuns(ctx)
	if err != nil {
		claims.release()
		return nil, fmt.Errorf("runs: reload claimed recovery Runs: %w", err)
	}
	claims.active = make([]rundomain.Run, 0, len(current))
	for _, candidate := range current {
		if claims.includes(candidate.SessionID()) {
			claims.active = append(claims.active, candidate)
		}
	}
	return claims, nil
}
