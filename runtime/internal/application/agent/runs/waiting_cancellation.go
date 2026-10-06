package runs

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// NewPreparedWaitingSubtreeCancellation captures one validated executor change
// and ownership-independent projections of its resulting waiting tree.
func NewPreparedWaitingSubtreeCancellation(
	canceledMemberIDs []string,
	pendingInterruptions []MemberInterruption,
	checkpoint ExecutorCheckpoint,
	change WaitingSubtreeChange,
) (PreparedWaitingSubtreeCancellation, error) {
	prepared := PreparedWaitingSubtreeCancellation{
		canceledMemberIDs:    slices.Clone(canceledMemberIDs),
		pendingInterruptions: cloneMemberInterruptions(pendingInterruptions),
		checkpoint:           checkpoint.Clone(),
		change:               change,
	}
	if err := prepared.validate(); err != nil {
		return PreparedWaitingSubtreeCancellation{}, err
	}
	return prepared, nil
}

// CanceledMemberIDs returns the exact canceled executor members.
func (p PreparedWaitingSubtreeCancellation) CanceledMemberIDs() []string {
	return slices.Clone(p.canceledMemberIDs)
}

// PendingInterruptions returns ownership-independent surviving input boundaries.
func (p PreparedWaitingSubtreeCancellation) PendingInterruptions() []MemberInterruption {
	return cloneMemberInterruptions(p.pendingInterruptions)
}

// Checkpoint returns an ownership-independent resulting executor snapshot.
func (p PreparedWaitingSubtreeCancellation) Checkpoint() ExecutorCheckpoint {
	return p.checkpoint.Clone()
}

// Apply installs the committed product disposition in the prepared executor tree.
func (p PreparedWaitingSubtreeCancellation) Apply(disposition WaitingSubtreeDisposition) error {
	if p.change == nil {
		return errors.New("runs: apply malformed prepared waiting subtree cancellation")
	}
	return p.change.Apply(disposition)
}

// Continue advances a prepared tree whose final waiting boundary was removed.
func (p PreparedWaitingSubtreeCancellation) Continue(ctx context.Context) error {
	if p.change == nil {
		return errors.New("runs: continue malformed prepared waiting subtree cancellation")
	}
	return p.change.Continue(ctx)
}

// Discard releases a prepared executor change that was not applied.
func (p PreparedWaitingSubtreeCancellation) Discard() error {
	if p.change == nil {
		return errors.New("runs: discard malformed prepared waiting subtree cancellation")
	}
	return p.change.Discard()
}

// validate verifies the Application projection and one-shot executor
// capability without interpreting the opaque checkpoint payload.
func (p PreparedWaitingSubtreeCancellation) validate() error {
	if dependency.Missing(p.change) {
		return errors.New("runs: prepared waiting subtree cancellation has no executor change")
	}
	if err := p.checkpoint.Validate(); err != nil {
		return err
	}
	if len(p.canceledMemberIDs) == 0 {
		return errors.New("runs: prepared waiting subtree cancellation has no canceled members")
	}
	canceledMembers := make(map[string]struct{}, len(p.canceledMemberIDs))
	for _, memberID := range p.canceledMemberIDs {
		if err := runtimeidentity.ValidateMember(memberID); err != nil {
			return fmt.Errorf("runs: prepared waiting subtree cancellation: %w", err)
		}
		if _, duplicate := canceledMembers[memberID]; duplicate {
			return fmt.Errorf("runs: prepared waiting subtree cancellation repeats member %q", memberID)
		}
		canceledMembers[memberID] = struct{}{}
	}
	requests := make(map[inputRequestKey]struct{}, len(p.pendingInterruptions))
	for index, interruption := range p.pendingInterruptions {
		if err := runtimeidentity.ValidateMember(interruption.MemberID); err != nil {
			return fmt.Errorf("runs: prepared waiting subtree interruption[%d]: %w", index, err)
		}
		if err := runtimeidentity.ValidateRequest(interruption.RequestID); err != nil {
			return fmt.Errorf("runs: prepared waiting subtree interruption[%d]: %w", index, err)
		}
		if _, canceled := canceledMembers[interruption.MemberID]; canceled {
			return fmt.Errorf(
				"runs: prepared waiting subtree interruption[%d] belongs to canceled member %q",
				index,
				interruption.MemberID,
			)
		}
		if err := interruption.Interrupt.Validate(); err != nil {
			return fmt.Errorf("runs: prepared waiting subtree interruption[%d]: %w", index, err)
		}
		identity := inputRequestIdentity(interruption.MemberID, interruption.RequestID)
		if _, duplicate := requests[identity]; duplicate {
			return fmt.Errorf(
				"runs: prepared waiting subtree cancellation repeats interruption %q/%q",
				identity.memberID,
				identity.requestID,
			)
		}
		requests[identity] = struct{}{}
	}
	return nil
}

type waitingCancellationTransformation struct {
	terminalRuns   []rundomain.Replacement
	terminalItems  []transcript.Replacement
	remaining      *Pending
	continuation   *treeContinuation
	checkpoint     ExecutorCheckpoint
	parked         []rundomain.Run
	targetRunID    string
	canceledRunIDs []string
}

// waitingCancellationBuilder owns the pure Application transformation from a
// command-bound cancellation plan and an executor-prepared change to the one
// durable write-set. It does not execute the one-shot change or interpret its
// opaque checkpoint.
type waitingCancellationBuilder struct {
	plan       cancellationPlan
	reason     string
	finishedAt time.Time
	prepared   PreparedWaitingSubtreeCancellation
}

func prepareWaitingCancellationTransformation(
	plan cancellationPlan,
	reason string,
	finishedAt time.Time,
	prepared PreparedWaitingSubtreeCancellation,
) (waitingCancellationTransformation, error) {
	builder := waitingCancellationBuilder{
		plan: plan, reason: reason, finishedAt: finishedAt, prepared: prepared,
	}
	return builder.build()
}

func (w waitingCancellationBuilder) build() (waitingCancellationTransformation, error) {
	if err := w.validate(); err != nil {
		return waitingCancellationTransformation{}, err
	}
	canceledMembers := make(map[string]struct{}, len(w.prepared.canceledMemberIDs))
	for _, memberID := range w.prepared.canceledMemberIDs {
		canceledMembers[memberID] = struct{}{}
	}

	terminalRuns, canceledRunIDs, err := w.terminalProjection(canceledMembers)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	terminalItems, continuations, err := w.settleWaitingItems(canceledMembers)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	kept, bindings, err := w.remainingInterruptions(canceledMembers, continuations)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	interrupts := make([]transcript.Interrupt, len(kept))
	open := make([]OpenInterrupt, len(kept))
	for position, index := range kept {
		interrupts[position], open[position] = w.plan.interrupts[index], w.plan.pending.Interrupts[index]
	}
	continuation, err := w.treeContinuation(interrupts, continuations)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	remaining, err := w.remainingPending(open, bindings, continuations)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	return waitingCancellationTransformation{
		terminalRuns:   terminalRuns,
		terminalItems:  terminalItems,
		remaining:      remaining,
		continuation:   continuation,
		checkpoint:     w.prepared.checkpoint.Clone(),
		parked:         w.plan.treeRuns(),
		targetRunID:    w.plan.target.run.ID(),
		canceledRunIDs: canceledRunIDs,
	}, nil
}

func (w waitingCancellationBuilder) validate() error {
	switch {
	case w.plan.treeState != rundomain.Waiting:
		return fmt.Errorf(
			"runs: waiting cancellation plan is %s",
			w.plan.treeState,
		)
	case !w.plan.target.run.Lineage().IsChild():
		return errors.New("runs: waiting cancellation target is not a child Run")
	case !w.plan.hasPending:
		return errors.New("runs: waiting cancellation plan has no pending set")
	case !w.plan.hasSpawningItem:
		return errors.New("runs: waiting cancellation plan has no spawning Item")
	case w.finishedAt.IsZero():
		return errors.New("runs: waiting cancellation finish time is required")
	}
	rootContinuation, ok := w.plan.pending.RootContinuation()
	if !ok {
		return errors.New("runs: waiting cancellation Pending has no root continuation")
	}
	if err := w.prepared.checkpoint.ValidateOwnership(
		rootContinuation.MemberID,
		w.plan.root.run.SessionID(),
	); err != nil {
		return fmt.Errorf("runs: invalid prepared waiting subtree checkpoint ownership: %w", err)
	}
	return nil
}

func (w waitingCancellationBuilder) terminalProjection(
	canceledMembers map[string]struct{},
) ([]rundomain.Replacement, []string, error) {
	expectedProcesses := make(map[string]struct{})
	var terminalRuns []rundomain.Replacement
	var canceledRunIDs []string
	for _, member := range w.plan.targetSubtree {
		if member.run.State().IsTerminal() {
			continue
		}
		if !member.hasMember {
			return nil, nil, fmt.Errorf(
				"runs: waiting cancellation target Run %q has no executor member",
				member.run.ID(),
			)
		}
		expectedProcesses[member.memberID] = struct{}{}
		replacement, err := rundomain.Replace(member.run, func(run rundomain.Run) (rundomain.Run, error) {
			return canceledWaitingRun(run, w.reason, w.finishedAt)
		})
		if err != nil {
			return nil, nil, err
		}
		terminalRuns = append(terminalRuns, replacement)
		canceledRunIDs = append(canceledRunIDs, member.run.ID())
	}
	if len(canceledMembers) != len(expectedProcesses) {
		return nil, nil, fmt.Errorf(
			"runs: prepared waiting cancellation removed %d members, Run subtree requires %d",
			len(canceledMembers),
			len(expectedProcesses),
		)
	}
	for memberID := range expectedProcesses {
		if _, canceled := canceledMembers[memberID]; !canceled {
			return nil, nil, fmt.Errorf(
				"runs: prepared waiting cancellation did not remove member %q",
				memberID,
			)
		}
	}
	return terminalRuns, canceledRunIDs, nil
}

func (w waitingCancellationBuilder) settleWaitingItems(
	canceledMembers map[string]struct{},
) ([]transcript.Replacement, []Continuation, error) {
	// The spawning Tool is deliberately left open. Its model-visible result
	// belongs to the round the parent declared, and only the executor can place it
	// there in that round's call order, so it stays a drained Tool until the
	// resumed Segment settles it alongside its siblings.
	parentItem := w.plan.spawningItem
	terminalItems := make([]transcript.Replacement, 0, len(w.plan.targetInterruptItems)+len(w.plan.targetDrainedItems))
	toolItems := slices.Clone(w.plan.targetDrainedItems)
	for _, item := range w.plan.targetInterruptItems {
		if item.Kind() == transcript.ToolCall {
			toolItems = append(toolItems, item)
		}
	}
	for _, item := range toolItems {
		itemFailure := tool.Failure{
			Kind:   tool.FailureExecution,
			Detail: w.reason,
		}
		itemReplacement, err := transcript.Replace(item, func(item transcript.Item) (transcript.Item, error) {
			return item.AbandonToolCall(&itemFailure, w.finishedAt)
		})
		if err != nil {
			return nil, nil, fmt.Errorf("runs: settle waiting Item %q: %w", item.ID(), err)
		}
		terminalItems = append(terminalItems, itemReplacement)
	}

	continuations := make([]Continuation, 0, len(w.plan.survivingTree))
	parentToolRetained := false
	for _, continuation := range w.plan.pending.Continuations {
		if _, canceled := canceledMembers[continuation.MemberID]; canceled {
			continue
		}
		clone := continuation
		clone.DrainedTools = slices.Clone(continuation.DrainedTools)
		if continuation.RunID == w.plan.target.run.Lineage().ParentRunID {
			var matches []DrainedTool
			for _, tool := range clone.DrainedTools {
				if tool.ItemID == parentItem.ID() {
					matches = append(matches, tool)
				}
			}
			if len(matches) != 1 {
				return nil, nil, fmt.Errorf(
					"runs: parent Run %q continuation has %d drained tools for spawning Item %q",
					continuation.RunID,
					len(matches),
					parentItem.ID(),
				)
			}
			parentToolRetained = true
		}
		continuations = append(continuations, clone)
	}
	if !parentToolRetained {
		return nil, nil, fmt.Errorf(
			"runs: waiting cancellation lost the drained spawning Item %q",
			parentItem.ID(),
		)
	}
	return terminalItems, continuations, nil
}

// remainingInterruptions returns, in the surviving order, the index of every
// kept interrupt in the expected Pending and its binding.
func (w waitingCancellationBuilder) remainingInterruptions(
	canceledMembers map[string]struct{},
	continuations []Continuation,
) ([]int, []InterruptBinding, error) {
	oldBindingByKey := make(map[inputRequestKey]int, len(w.plan.pending.Bindings))
	for index, binding := range w.plan.pending.Bindings {
		oldBindingByKey[inputRequestIdentity(binding.MemberID, binding.RequestID)] = index
	}
	survivingRunByMemberID := make(map[string]string, len(continuations))
	for _, continuation := range continuations {
		survivingRunByMemberID[continuation.MemberID] = continuation.RunID
	}
	pendingInterruptions := w.prepared.pendingInterruptions
	remainingInterrupts := make([]int, 0, len(pendingInterruptions))
	remainingBindings := make([]InterruptBinding, 0, len(pendingInterruptions))
	keptBindings := make(map[int]struct{}, len(pendingInterruptions))
	for _, boundary := range pendingInterruptions {
		if err := boundary.Interrupt.Validate(); err != nil {
			return nil, nil, fmt.Errorf(
				"runs: prepared member %q input request %q: %w",
				boundary.MemberID,
				boundary.RequestID,
				err,
			)
		}
		index, exists := oldBindingByKey[inputRequestIdentity(boundary.MemberID, boundary.RequestID)]
		if !exists {
			return nil, nil, fmt.Errorf(
				"runs: prepared member %q input request %q was absent from the durable pending set",
				boundary.MemberID,
				boundary.RequestID,
			)
		}
		if _, duplicate := keptBindings[index]; duplicate {
			return nil, nil, fmt.Errorf(
				"runs: prepared member %q repeated input request %q",
				boundary.MemberID,
				boundary.RequestID,
			)
		}
		binding := w.plan.pending.Bindings[index]
		interrupt := w.plan.interrupts[index]
		runID, survives := survivingRunByMemberID[binding.MemberID]
		if !survives || interrupt.RunID != runID {
			return nil, nil, fmt.Errorf(
				"runs: prepared input request %q belongs to removed member %q",
				binding.RequestID,
				binding.MemberID,
			)
		}
		if interrupt.Kind != boundary.Interrupt.Kind {
			return nil, nil, fmt.Errorf(
				"runs: prepared input request %q changed interrupt kind from %s to %s",
				binding.RequestID,
				interrupt.Kind,
				boundary.Interrupt.Kind,
			)
		}
		keptBindings[index] = struct{}{}
		remainingInterrupts = append(remainingInterrupts, index)
		remainingBindings = append(remainingBindings, binding)
	}
	for index, binding := range w.plan.pending.Bindings {
		if _, kept := keptBindings[index]; kept {
			continue
		}
		if _, canceled := canceledMembers[binding.MemberID]; !canceled {
			return nil, nil, fmt.Errorf(
				"runs: prepared cancellation dropped surviving member %q input request %q",
				binding.MemberID,
				binding.RequestID,
			)
		}
	}
	return remainingInterrupts, remainingBindings, nil
}

func (w waitingCancellationBuilder) treeContinuation(
	interrupts []transcript.Interrupt,
	continuations []Continuation,
) (*treeContinuation, error) {
	continuation := &treeContinuation{
		rootRunID:     w.plan.pending.RootRunID,
		executorID:    w.plan.pending.ExecutorID,
		interrupts:    slices.Clone(interrupts),
		continuations: slices.Clone(continuations),
		runs:          parkedRunsByID(w.plan.survivingRuns()),
		items:         maps.Clone(w.plan.items),
	}
	if err := continuation.validate(); err != nil {
		return nil, fmt.Errorf(
			"runs: waiting cancellation continuation: %w",
			err,
		)
	}
	return continuation, nil
}

func (w waitingCancellationBuilder) remainingPending(
	interrupts []OpenInterrupt,
	bindings []InterruptBinding,
	continuations []Continuation,
) (*Pending, error) {
	if len(interrupts) == 0 {
		return nil, nil
	}
	reduced := w.plan.pending
	reduced.Interrupts = interrupts
	reduced.Bindings = bindings
	reduced.Continuations = continuations
	if err := reduced.Validate(); err != nil {
		return nil, fmt.Errorf(
			"runs: reduced waiting cancellation pending set: %w",
			err,
		)
	}
	return &reduced, nil
}

func canceledWaitingRun(run rundomain.Run, reason string, finishedAt time.Time) (rundomain.Run, error) {
	terminal, err := run.CancelWaiting(reason, finishedAt, rundomain.UnknownMessageMark())
	if err != nil {
		return rundomain.Run{}, fmt.Errorf("runs: cancel waiting Run %q: %w", run.ID(), err)
	}
	return terminal, nil
}

type inputRequestKey struct {
	memberID  string
	requestID string
}

func inputRequestIdentity(memberID, requestID string) inputRequestKey {
	return inputRequestKey{memberID: memberID, requestID: requestID}
}

func (w waitingCancellationTransformation) durableCommit(
	expected Pending,
	commitID runtimeidentity.CommitID,
) (WaitingSubtreeCancellationCommit, error) {
	if w.remaining == nil {
		return WaitingSubtreeCancellationCommit{}, errors.New("runs: waiting cancellation has no reduced Pending")
	}
	return NewParkedSubtreeCancellationCommit(
		commitID, w.targetRunID, w.parked, expected, *w.remaining,
		w.checkpoint, w.terminalRuns, w.terminalItems,
	)
}

func (w waitingCancellationTransformation) resumedDurableCommit(
	expected Pending,
	commitID runtimeidentity.CommitID,
	resume rundomain.TreeResumeDraft,
	openingEvents []EventCommit,
) (WaitingSubtreeCancellationCommit, error) {
	return NewResumingSubtreeCancellationCommit(
		commitID, w.targetRunID, w.parked, expected, w.checkpoint,
		w.terminalRuns, w.terminalItems,
		resume, openingEvents,
	)
}
