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
	targetRunID    string
	parentRunID    string
	spawningItemID string
	reason         string
	root           rundomain.Run
	executor       ExecutorRef
	record         ChildCancellationReceipt
	targetRunIDs   map[string]struct{}
	targetTerminal *rundomain.Run
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
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		if !member.run.State().IsTerminal() {
			targetRunIDs[member.run.ID()] = struct{}{}
		}
	}
	attempt := &childCancellation{
		targetRunID:    plan.target.run.ID(),
		parentRunID:    plan.target.run.Lineage().ParentRunID(),
		spawningItemID: plan.target.run.Lineage().SpawnedByItemID(),
		reason:         reason,
		root:           plan.root.run,
		executor:       plan.executor,
		record:         record,
		targetRunIDs:   targetRunIDs,
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
	case r.terminalChildren != nil:
		if terminal, finished := r.terminalChildren[attempt.targetRunID]; finished {
			return nil, fmt.Errorf(
				"%w: %q completed as %s",
				ErrRunFinished,
				attempt.targetRunID,
				terminal.State(),
			)
		}
	}
	r.childCancel = attempt
	return attempt, nil
}

func (r *runTreeOwner) abortChildCancellation(attempt *childCancellation, err error) {
	if attempt == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.childCancel != attempt || attempt.finished() {
		return
	}
	attempt.err = err
	r.finishChildCancellationLocked(attempt)
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
		attempt.spawningItemID != itemID ||
		attempt.terminalBefore(prior) == nil {
		return event
	}
	classified := event
	classified.Failure = &tool.Failure{
		Kind:   tool.FailureChildRunCanceled,
		Detail: attempt.reason,
	}
	return classified
}

func (c *childCancellation) terminalBefore(prior []EventCommit) *rundomain.Run {
	if c.targetTerminal != nil {
		return c.targetTerminal
	}
	for _, staged := range prior {
		if staged.RunID == c.targetRunID && staged.State == StateTerminalize && staged.Run.Canceled() {
			return staged.Run
		}
	}
	return nil
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
		attempt.targetTerminal == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != item.ID() {
		return
	}
	// A canceled child may retain unknown Effects. Its parent then abandons
	// the open Item without claiming a definite Tool failure or model result.
	if item.Status() != transcript.ItemIncomplete {
		attempt.err = fmt.Errorf("runs: canceled child Run %q parent item %q committed in status %s", attempt.targetRunID, item.ID(), item.Status())
	}

	r.finishChildCancellationLocked(attempt)
}

// waitChildCancellation returns the canceled target and its original root snapshot.
// recordTerminalRun admits only the target's Canceled terminal, and every other
// path that closes the attempt records why, so a joined attempt without an
// error always carries that snapshot.
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
	return attempt.result(*attempt.targetTerminal), nil
}

// The root reply is available once teardown joins. A child cancellation still
// needs its spawning Tool result, so retain the child snapshot until then.
func (r *runTreeOwner) recordTerminalRun(run rundomain.Run, cancellation *rootCancellation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.Lineage().IsRoot() {
		if r.rootTerminal != nil {
			panic("runs: live segment committed more than one root terminal snapshot")
		}
		r.rootTerminal = &rootTerminal{run: run}
		if cancellation != nil && run.Canceled() {
			r.rootTerminal.cancellation = cancellation.id
		}
		return
	}
	if r.terminalChildren == nil {
		r.terminalChildren = make(map[string]rundomain.Run)
	}
	if _, duplicate := r.terminalChildren[run.ID()]; duplicate {
		panic(fmt.Sprintf("runs: Run %q committed more than one terminal snapshot", run.ID()))
	}
	r.terminalChildren[run.ID()] = run
	attempt := r.childCancel
	if attempt == nil || run.ID() != attempt.targetRunID {
		return
	}
	if !run.Canceled() {
		attempt.err = fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			run.ID(),
			run.State(),
		)
		r.finishChildCancellationLocked(attempt)
		return
	}
	terminal := run
	attempt.targetTerminal = &terminal
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

// CancelReasonFor returns the reason owned by runID's winning cancellation
// plan. A child operation applies its reason only to the target subtree; the
// root reason remains independent for a later whole-tree cancellation.
func (r *runTreeOwner) CancelReasonFor(runID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.childCancel != nil {
		if _, targeted := r.childCancel.targetRunIDs[runID]; targeted {
			return r.childCancel.reason
		}
	}
	if r.rootCancel == nil {
		return ""
	}
	return r.rootCancel.reason
}
