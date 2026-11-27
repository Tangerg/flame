package runs

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type recoveryPlanner struct {
	independent   []modelref.EndpointObservation
	ctx           context.Context
	store         RecoveryStore
	resumability  WaitingExecutionResumability
	pendingByRoot map[identity.RunID]Pending
	trees         map[identity.RunID]recoveryRunTree
	transcripts   map[identity.SessionID][]transcript.Item
	commit        RecoveryCommit
	finishedAt    time.Time
}

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
		pendingByRoot: pendingByRun,
		trees:         trees,
		transcripts:   make(map[identity.SessionID][]transcript.Item),
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
		planner.commit.modelInvocations = append(planner.commit.modelInvocations, ModelInvocationRecovery{OpenModelInvocation: invocation, FinishedAt: planner.finishedAt})
	}
	for _, invocation := range snapshot.toolInvocations {
		planner.commit.toolInvocations = append(planner.commit.toolInvocations, ToolInvocationRecovery{OpenToolInvocation: invocation, FinishedAt: planner.finishedAt})
	}
	return planner, nil
}

// Wall time can move backward across a reboot, so the recovery timestamp derives
// from the boot snapshot.
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

func (r *recoveryPlanner) plan() (RecoveryCommit, error) {
	trees := slices.SortedFunc(maps.Values(r.trees), func(left, right recoveryRunTree) int {
		return left.root.SessionID().Compare(right.root.SessionID())
	})
	for _, tree := range trees {
		outcome, err := r.planTree(tree)
		if err != nil {
			return RecoveryCommit{}, err
		}
		r.commit.trees = append(r.commit.trees, outcome)
		if _, lost := outcome.(LostTreeRecovery); lost {
			for _, source := range r.independent {
				if _, belongs := tree.runsByID[source.Snapshot().RunID]; belongs {
					r.commit.independentEndpoints = append(r.commit.independentEndpoints, source)
				}
			}
		}
	}
	slices.SortFunc(r.commit.modelInvocations, compareModelInvocationRecoveries)
	slices.SortFunc(r.commit.toolInvocations, compareToolInvocationRecoveries)
	if err := r.commit.validateInvocationSources(); err != nil {
		return RecoveryCommit{}, err
	}
	return r.commit, nil
}

func compareModelInvocationRecoveries(left, right ModelInvocationRecovery) int {
	return compareInvocationAttempts(left.InvocationAttempt, right.InvocationAttempt)
}

func compareToolInvocationRecoveries(left, right ToolInvocationRecovery) int {
	return cmp.Or(compareInvocationAttempts(left.InvocationAttempt, right.InvocationAttempt), left.ItemID.Compare(right.ItemID))
}

func (r *recoveryPlanner) planTree(tree recoveryRunTree) (recoveryTreeOutcome, error) {
	root := tree.root
	items, err := r.transcript(tree)
	if err != nil {
		return nil, err
	}
	open, hasInterrupt := r.pendingByRoot[root.ID()]
	loss := rundomain.LossRestart
	if root.State() == rundomain.Waiting && hasInterrupt {
		sess, sessionErr := r.session(root.SessionID())
		if sessionErr != nil {
			return nil, sessionErr
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
			return nil, sessionErr
		}
		if resumption.Resumable() || resumption.Deferred() {
			return preservedRecoveryTree{session: root.SessionID()}, nil
		}
		loss = resumption.Loss()
	}
	history, err := r.conversation(root.SessionID())
	if err != nil {
		return nil, err
	}
	ending := rundomain.Lost{Loss: rundomain.Loss{Reason: loss}}
	messages, err := TerminalConversation(r.ctx, r.store, root.SessionID(), root.ID(), history.Messages(), ending)
	if err != nil {
		return nil, fmt.Errorf(
			"runs: close recovery conversation for root Run %q: %w",
			root.ID(),
			err,
		)
	}
	closure := recoveryConversationClosure{previousCount: history.Count(), messages: messages}
	return recoverLostTree(tree, items, closure, r.finishedAt, ending, r.commit.modelInvocations, r.independent)
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
	sess, err := r.store.SessionByID(r.ctx, sessionID)
	if err != nil {
		return session.Session{}, fmt.Errorf("runs: load recovery Session %q: %w", sessionID, err)
	}
	return sess, nil
}

func (r *recoveryPlanner) conversation(sessionID identity.SessionID) (conversation.Conversation, error) {
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
	return history, nil
}
