package runs

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (r *runTreeOwner) admitCancellation(root rundomain.Run, record Record) error {
	if record.ID != root.ID() || record.ExecutorID == "" {
		return fmt.Errorf("runs: cancellation root %q has an invalid live executor binding", root.ID())
	}
	r.mu.Lock()
	parking := r.interrupt.committed || r.interrupt.active != nil
	r.mu.Unlock()
	if root.State() == rundomain.Waiting && parking {
		return fmt.Errorf("%w: Run %q is completing parked maintenance", rundomain.ErrSessionBusy, root.ID())
	}
	if root.State() != rundomain.Running || record.SegmentID != root.ActiveSegmentID() {
		return fmt.Errorf(
			"runs: cancellation root %q durable segment %q differs from live owner %q",
			root.ID(), root.ActiveSegmentID(), record.SegmentID,
		)
	}
	return nil
}

// childCancellation is the root Run-tree owner's one active child-cancel boundary.
// The owner admits either this boundary or whole-tree cancellation, never both.
type childCancellation struct {
	id             CancellationID
	targetRunID    string
	parentRunID    string
	spawningItemID string
	reason         string
	root           rundomain.Run
	executor       ExecutorRef
	record         ChildCancellationReceipt
	done           chan struct{}
	err            error
}

func (c *childCancellation) finished() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (r *runTreeOwner) beginChildCancellation(
	plan cancellationPlan,
	reason string,
	record ChildCancellationReceipt,
) (*childCancellation, error) {
	if !plan.target.run.Lineage().IsChild() {
		return nil, fmt.Errorf("runs: cancellation target %q is not a child Run", plan.target.run.ID())
	}
	if plan.treeState() != rundomain.Running {
		return nil, fmt.Errorf(
			"runs: live child cancellation requires a running tree, got %s",
			plan.treeState(),
		)
	}
	if !plan.target.hasMember {
		return nil, fmt.Errorf(
			"runs: child cancellation target %q has no executor member binding",
			plan.target.run.ID(),
		)
	}
	attempt := &childCancellation{
		id:             newCancellationID(),
		targetRunID:    plan.target.run.ID(),
		parentRunID:    plan.target.run.Lineage().ParentRunID(),
		spawningItemID: plan.target.run.Lineage().SpawnedByItemID(),
		reason:         reason,
		root:           plan.root.run,
		executor:       plan.executor,
		record:         record,
		done:           make(chan struct{}),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.rootCancel != nil:
		return nil, fmt.Errorf(
			"%w: root Run %q cancellation owns the tree",
			rundomain.ErrSessionBusy,
			plan.root.run.ID(),
		)
	case r.childCancel != nil:
		return nil, fmt.Errorf(
			"%w: child Run %q cancellation owns the tree",
			rundomain.ErrSessionBusy,
			r.childCancel.targetRunID,
		)
	}
	if terminal, finished := r.terminalRuns[attempt.targetRunID]; finished {
		ending, _ := terminal.run.Ending()
		return nil, fmt.Errorf("%w: %q settled as %s", ErrRunFinished, attempt.targetRunID, ending.Outcome())
	}
	r.childCancel = attempt
	return attempt, nil
}

func (r *runTreeOwner) rejectChildCancellation(attempt *childCancellation, err error) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.childCancel != attempt || attempt.finished() {
		return false
	}
	if _, confirmed := r.childCancellationTerminalLocked(attempt, nil); confirmed {
		return false
	}
	attempt.err = err
	r.finishChildCancellationLocked(attempt)
	return true
}

func (r *runTreeOwner) finishChildCancellationLocked(attempt *childCancellation) {
	if attempt == nil || attempt.finished() {
		return
	}
	if r.childCancel == attempt {
		r.childCancel = nil
	}
	close(attempt.done)
}

func (r *runTreeOwner) classifyChildCancellationTool(
	parentRunID string,
	itemID string,
	event ToolCallFinished,
	prior []EventCommit,
) ToolCallFinished {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.childCancel
	if attempt == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != itemID {
		return event
	}
	if _, confirmed := r.childCancellationTerminalLocked(attempt, prior); !confirmed {
		return event
	}
	classified := event
	classified.Failure = &tool.Failure{
		Kind:   tool.FailureChildRunCanceled,
		Detail: attempt.reason,
	}
	return classified
}

func (r *runTreeOwner) childCancellationTerminalLocked(attempt *childCancellation, prior []EventCommit) (rundomain.Run, bool) {
	if terminal := r.terminalRuns[attempt.targetRunID]; terminal.cancellation == attempt.id {
		return terminal.run, true
	}
	for _, staged := range prior {
		if staged.RunID == attempt.targetRunID && staged.State == StateTerminalize && staged.cancellation == attempt.id {
			return *staged.Run, true
		}
	}
	return rundomain.Run{}, false
}

func (c *childCancellation) result(terminal rundomain.Run) CancelResult {
	root := c.root
	return CancelResult{Run: terminal, RootRun: &root}
}

func (r *runTreeOwner) recordChildCancellationItem(parentRunID string, item transcript.Item) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.childCancel
	if attempt == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != item.ID() {
		return
	}
	if _, confirmed := r.childCancellationTerminalLocked(attempt, nil); !confirmed {
		return
	}
	if item.Status() != transcript.ItemIncomplete {
		attempt.err = fmt.Errorf("runs: canceled child Run %q parent item %q committed in status %s", attempt.targetRunID, item.ID(), item.Status())
	}

	r.finishChildCancellationLocked(attempt)
}

func (r *runTreeOwner) waitChildCancellation(
	ctx context.Context,
	attempt *childCancellation,
) (CancelResult, error) {
	select {
	case <-attempt.done:
	case <-r.done:
		select {
		case <-attempt.done:
		default:
			if r.completionErr != nil {
				return CancelResult{}, r.completionErr
			}
			return CancelResult{}, fmt.Errorf(
				"runs: root segment ended before child Run %q cancellation committed its parent result",
				attempt.targetRunID,
			)
		}
	case <-ctx.Done():
		return CancelResult{}, context.Cause(ctx)
	}
	if attempt.err != nil {
		return CancelResult{}, attempt.err
	}
	terminal, found := r.committedRunTerminal(attempt.targetRunID)
	if !found || terminal.cancellation != attempt.id {
		return CancelResult{}, fmt.Errorf("runs: child Run %q has no terminal for its cancellation", attempt.targetRunID)
	}
	return attempt.result(terminal.run), nil
}

func (r *runTreeOwner) recordTerminalRun(run rundomain.Run, cancellation CancellationID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalRuns == nil {
		r.terminalRuns = make(map[string]terminalRun)
	}
	if _, duplicate := r.terminalRuns[run.ID()]; duplicate {
		panic(fmt.Sprintf("runs: Run %q committed more than one terminal snapshot", run.ID()))
	}
	r.terminalRuns[run.ID()] = terminalRun{run: run, cancellation: cancellation}
	attempt := r.childCancel
	if attempt == nil || run.ID() != attempt.targetRunID {
		return
	}
	if cancellation != attempt.id {
		ending, _ := run.Ending()
		attempt.err = fmt.Errorf("%w: %q settled as %s", ErrRunFinished, run.ID(), ending.Outcome())
		r.finishChildCancellationLocked(attempt)
	}
}

// interruptBoundary records the only two durable facts cancellation needs:
// whether publication committed, and whether one publication is currently
// owned. Both fields are guarded by runTreeOwner.mu.
type interruptBoundary struct {
	committed bool
	active    chan struct{}
}

// Once an interrupt begins its own transaction, cancellation joins that
// boundary before choosing the live executor or the parked Run.
func (r *runTreeOwner) commitInterrupt(ctx context.Context, commit func(context.Context) error) (committed bool, err error) {
	r.mu.Lock()
	if r.rootCancel != nil {
		r.mu.Unlock()
		return false, nil
	}
	if r.interrupt.committed || r.interrupt.active != nil {
		r.mu.Unlock()
		return false, errors.New("runs: interrupt boundary already resolved")
	}
	inflight := make(chan struct{})
	r.interrupt.active = inflight
	r.mu.Unlock()

	err = commit(ctx)
	r.mu.Lock()
	if err == nil {
		r.interrupt.committed = true
	}
	close(inflight)
	r.interrupt.active = nil
	r.mu.Unlock()
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *runTreeOwner) cancellationReason(id CancellationID, child bool) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if child && r.childCancel != nil && r.childCancel.id == id {
		return r.childCancel.reason, true
	}
	if r.rootCancel != nil && r.rootCancel.id == id {
		return r.rootCancel.reason, true
	}
	return "", false
}

func (r *runTreeOwner) projectCancellation(fact ExecutionFact, child bool) (ExecutionFact, CancellationID) {
	end, ok := fact.(SegmentEnded)
	if !ok || end.cancellation.IsZero() {
		return fact, CancellationID{}
	}
	reason, matched := r.cancellationReason(end.cancellation, child)
	if !matched {
		return fact, CancellationID{}
	}
	end.Ending = rundomain.Canceled{Detail: reason}
	return end, end.cancellation
}
