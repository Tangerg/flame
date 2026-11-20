package runs

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// recoveryPlanner owns one ownership-scoped reconciliation snapshot and the caches needed
// to derive its atomic write-set. It is intentionally Application-private:
// deciding whether an opaque checkpoint preserves a product Run is a recovery
// policy, not a Run aggregate or executor concern.
type recoveryPlanner struct {
	independent   []modelref.EndpointObservation
	ctx           context.Context
	store         RecoveryStore
	resumability  WaitingExecutionResumability
	pending       []Pending
	pendingByRoot map[identity.RunID]Pending
	trees         map[identity.RunID]recoveryRunTree
	transcripts   map[identity.SessionID][]transcript.Item
	sessions      map[identity.SessionID]session.Session
	conversations map[identity.SessionID]conversation.Conversation
	preserved     map[identity.RunID]struct{}
	commit        RecoveryCommitInput
	finishedAt    time.Time
}

// recoverySnapshot is the durable state recovery closes, narrowed to the
// Sessions this boot claimed.
type recoverySnapshot struct {
	pending          []Pending
	modelInvocations []OpenModelInvocation
	toolInvocations  []OpenToolInvocation
	independent      []modelref.EndpointObservation
}

func loadRecoverySnapshot(ctx context.Context, store RecoveryStore, claims *recoverySessionClaims) (recoverySnapshot, error) {
	pending, err := store.ListPendingInterrupts(ctx)
	if err != nil {
		return recoverySnapshot{}, fmt.Errorf("runs: load pending interrupts for recovery: %w", err)
	}
	modelInvocations, err := store.ListOpenModelInvocations(ctx)
	if err != nil {
		return recoverySnapshot{}, fmt.Errorf("runs: load open model invocations for recovery: %w", err)
	}
	independent, err := store.ListIndependentEndpoints(ctx)
	if err != nil {
		return recoverySnapshot{}, fmt.Errorf("runs: load independent model sources: %w", err)
	}
	toolInvocations, err := store.ListOpenToolInvocations(ctx)
	if err != nil {
		return recoverySnapshot{}, fmt.Errorf("runs: load open Tool invocations for recovery: %w", err)
	}
	activeRoots := claims.activeRoots()
	return recoverySnapshot{
		pending: slices.DeleteFunc(slices.Clone(pending), func(open Pending) bool {
			_, ok := activeRoots[open.RootRunID]
			return !ok
		}),
		modelInvocations: slices.DeleteFunc(slices.Clone(modelInvocations), func(open OpenModelInvocation) bool {
			return !claims.includes(open.SessionID)
		}),
		toolInvocations: slices.DeleteFunc(slices.Clone(toolInvocations), func(open OpenToolInvocation) bool {
			return !claims.includes(open.SessionID)
		}),
		independent: slices.DeleteFunc(slices.Clone(independent), func(source modelref.EndpointObservation) bool {
			return !claims.includes(source.Snapshot().Invocation.SessionID)
		}),
	}, nil
}

func newRecoveryPlanner(ctx context.Context, recovery *Recovery, claims *recoverySessionClaims) (*recoveryPlanner, error) {
	snapshot, err := loadRecoverySnapshot(ctx, recovery.store, claims)
	if err != nil {
		return nil, err
	}
	pendingByRun, err := indexRecoveryPending(snapshot.pending, claims.activeRoots())
	if err != nil {
		return nil, err
	}
	trees, err := groupRecoveryRunTrees(claims.active)
	if err != nil {
		return nil, err
	}
	if err := validateOpenInvocationOwners(trees, snapshot.modelInvocations, snapshot.toolInvocations); err != nil {
		return nil, err
	}
	planner := &recoveryPlanner{
		ctx:           ctx,
		store:         recovery.store,
		resumability:  recovery.resumability,
		independent:   snapshot.independent,
		pending:       snapshot.pending,
		pendingByRoot: pendingByRun,
		trees:         trees,
		transcripts:   make(map[identity.SessionID][]transcript.Item),
		sessions:      make(map[identity.SessionID]session.Session),
		conversations: make(map[identity.SessionID]conversation.Conversation),
		preserved:     make(map[identity.RunID]struct{}, len(trees)),
		finishedAt:    recovery.now().UTC(),
	}
	if err := planner.observeSnapshot(claims.active, snapshot); err != nil {
		return nil, err
	}
	// Every active tree's Transcript is required by planTree anyway. Preload it
	// before constructing any terminal facts so a later Item in one tree cannot
	// advance the shared recovery timestamp after another tree was planned.
	for _, tree := range trees {
		items, err := planner.transcript(tree)
		if err != nil {
			return nil, err
		}
		if err := validateRecoveryToolInvocationItems(tree, items, snapshot.toolInvocations); err != nil {
			return nil, err
		}
	}
	for _, invocation := range snapshot.modelInvocations {
		planner.commit.ModelInvocations = append(planner.commit.ModelInvocations, ModelInvocationRecovery{OpenModelInvocation: invocation, FinishedAt: planner.finishedAt})
	}
	for _, invocation := range snapshot.toolInvocations {
		planner.commit.ToolInvocations = append(planner.commit.ToolInvocations, ToolInvocationRecovery{OpenToolInvocation: invocation, FinishedAt: planner.finishedAt})
	}
	return planner, nil
}

// observeSnapshot advances the recovery timestamp past every fact it closes.
// Wall time can move backward across a reboot, so the observation derives from
// the complete boot snapshot instead of letting a regressed clock make
// otherwise-recoverable state permanently invalid.
func (r *recoveryPlanner) observeSnapshot(active []rundomain.Run, snapshot recoverySnapshot) error {
	for _, value := range active {
		r.observeTime(value.UpdatedAt())
	}
	for _, open := range snapshot.pending {
		r.observeTime(open.CreatedAt)
	}
	for _, invocation := range snapshot.modelInvocations {
		r.observeTime(invocation.StartedAt)
		observedAt, err := invocation.endpointObservedAt()
		if err != nil {
			return fmt.Errorf("runs: read endpoint observation for abandoned call %q: %w", invocation.CallID, err)
		}
		r.observeTime(observedAt)
	}
	for _, invocation := range snapshot.toolInvocations {
		r.observeTime(invocation.StartedAt)
	}
	for _, source := range snapshot.independent {
		r.observeTime(source.Snapshot().FinishedAt)
	}
	return nil
}

func (r *recoveryPlanner) observeTime(value time.Time) {
	value = value.UTC()
	if value.After(r.finishedAt) {
		r.finishedAt = value
	}
}

func (r *recoveryPlanner) plan() (RecoveryCommit, int, error) {
	rootRunIDs := make([]identity.RunID, 0, len(r.trees))
	for rootRunID := range r.trees {
		rootRunIDs = append(rootRunIDs, rootRunID)
	}
	slices.SortFunc(rootRunIDs, identity.RunID.Compare)
	for _, rootRunID := range rootRunIDs {
		if err := r.planTree(rootRunID); err != nil {
			return RecoveryCommit{}, 0, err
		}
	}
	for _, open := range r.pending {
		if _, preserved := r.preserved[open.RootRunID]; preserved {
			r.commit.PreservedSessionIDs = append(r.commit.PreservedSessionIDs, r.trees[open.RootRunID].root.SessionID())
		}
	}
	slices.SortFunc(r.commit.DeleteInterrupts, identity.RunID.Compare)
	slices.SortFunc(r.commit.ModelInvocations, compareModelInvocationRecoveries)
	slices.SortFunc(r.commit.ToolInvocations, compareToolInvocationRecoveries)
	slices.SortFunc(r.commit.PreservedSessionIDs, identity.SessionID.Compare)
	slices.SortFunc(r.commit.DeleteCheckpointSessionIDs, identity.SessionID.Compare)
	commit, err := NewRecoveryCommit(r.commit)
	if err != nil {
		return RecoveryCommit{}, 0, err
	}
	return commit, len(commit.LostRuns()), nil
}

func compareModelInvocationRecoveries(left, right ModelInvocationRecovery) int {
	return left.InvocationAttempt.compare(right.InvocationAttempt)
}

func compareToolInvocationRecoveries(left, right ToolInvocationRecovery) int {
	return cmp.Or(left.InvocationAttempt.compare(right.InvocationAttempt), left.ItemID.Compare(right.ItemID))
}

func (r *recoveryPlanner) planTree(rootRunID identity.RunID) error {
	tree := r.trees[rootRunID]
	items, err := r.transcript(tree)
	if err != nil {
		return err
	}
	open, hasInterrupt := r.pendingByRoot[rootRunID]
	loss := rundomain.LossRestart
	if tree.root.State() == rundomain.Waiting && hasInterrupt {
		sess, sessionErr := r.session(tree.root.SessionID())
		if sessionErr != nil {
			return sessionErr
		}
		resumption, sessionErr := validateRecoveryParkedTree(
			r.ctx,
			tree,
			open,
			sess,
			items,
			r.store,
			r.resumability,
		)
		if sessionErr != nil {
			return sessionErr
		}
		if resumption.Resumable() || resumption.Deferred() {
			r.preserved[rootRunID] = struct{}{}
			return nil
		}
		loss = resumption.Loss()
	}
	history, err := r.conversation(tree.root.SessionID())
	if err != nil {
		return err
	}
	closure, err := TerminalConversation(r.ctx, r.store, tree.root.SessionID(), tree.root.ID(), history.Messages(), rundomain.OutcomeLost, "")
	if err != nil {
		return fmt.Errorf(
			"runs: close recovery conversation for root Run %q: %w",
			tree.root.ID(),
			err,
		)
	}
	messageMark := history.Count() + len(closure)
	lostRuns, replacements, err := recoverLostTree(tree, items, messageMark, r.finishedAt, loss, r.commit.ModelInvocations, r.independent)
	if err != nil {
		return err
	}
	r.commit.LostRuns = append(r.commit.LostRuns, lostRuns...)
	for _, lost := range lostRuns {
		for _, source := range r.independent {
			if source.Snapshot().RunID == lost.State().ID() {
				r.commit.IndependentEndpoints = append(r.commit.IndependentEndpoints, source)
			}
		}
	}
	r.commit.ItemReplacements = append(r.commit.ItemReplacements, replacements...)
	r.commit.ConversationTransitions = append(
		r.commit.ConversationTransitions,
		RecoveryConversationTransition{
			RootRunID: tree.root.ID(), SessionID: tree.root.SessionID(),
			ExpectedCount: history.Count(), Messages: closure,
		},
	)
	r.commit.DeleteInterrupts = append(r.commit.DeleteInterrupts, tree.root.ID())
	r.commit.DeleteCheckpointSessionIDs = append(
		r.commit.DeleteCheckpointSessionIDs,
		tree.root.SessionID(),
	)
	return nil
}

func (r *recoveryPlanner) transcript(tree recoveryRunTree) ([]transcript.Item, error) {
	sessionID := tree.root.SessionID()
	if items, ok := r.transcripts[sessionID]; ok {
		return items, nil
	}
	items, err := r.store.ListTranscript(r.ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runs: load recovery transcript for Session %q: %w", sessionID, err)
	}
	if err := validateRecoveryTranscript(tree, items); err != nil {
		return nil, err
	}
	for _, item := range items {
		r.observeTime(item.OccurredAt())
		if call, ok := item.ToolCall(); ok {
			r.observeTime(call.FinishedAt)
		}
	}
	r.transcripts[sessionID] = items
	return items, nil
}

func (r *recoveryPlanner) session(sessionID identity.SessionID) (session.Session, error) {
	if sess, ok := r.sessions[sessionID]; ok {
		return sess, nil
	}
	sess, err := r.store.SessionByID(r.ctx, sessionID)
	if err != nil {
		return session.Session{}, fmt.Errorf("runs: load recovery Session %q: %w", sessionID, err)
	}
	r.sessions[sessionID] = sess
	return sess, nil
}

func (r *recoveryPlanner) conversation(sessionID identity.SessionID) (conversation.Conversation, error) {
	if history, ok := r.conversations[sessionID]; ok {
		return history, nil
	}
	messages, err := r.store.ReadMessages(r.ctx, sessionID)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf(
			"runs: load recovery conversation for Session %q: %w",
			sessionID,
			err,
		)
	}
	history, err := conversation.New(messages)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf(
			"runs: validate recovery conversation for Session %q: %w",
			sessionID,
			err,
		)
	}
	r.conversations[sessionID] = history
	return history, nil
}
