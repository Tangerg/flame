package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/samber/lo"
)

// CancellationSettlementError means the Run owner could not join its complete
// cleanup boundary. A committed terminal Run cannot turn it into a benign refusal.
type CancellationSettlementError struct{ Cause error }

func (e *CancellationSettlementError) Error() string {
	if e.Cause == nil {
		return "runs: cancellation settlement failed"
	}
	return fmt.Sprintf("runs: cancellation settlement failed: %v", e.Cause)
}

func (e *CancellationSettlementError) Unwrap() error { return e.Cause }

// Cancellations owns Run cancellation: a live root or child, a waiting child and
// a parked Run, including recovery of a committed waiting-child cancellation.
type Cancellations struct {
	rootCancellation                   RunningRootCancellationRequester
	runningSubtreeCanceler             RunningSubtreeCanceler
	waitingSubtreeCancellationPreparer WaitingSubtreeCancellationPreparer
	waitingSubtreeCancellations        WaitingSubtreeCancellationCommitter
	acceptedSubtreeCancellations       acceptedSubtreeCancellationStore
	subtreeCancellationRestorer        AcceptedSubtreeCancellationRestorer
	terminations                       terminationCommitter
	sessionReader                      sessionReader
	interrupts                         pendingInterruptReader
	checkpoints                        WaitingCheckpointReader
	releases                           ExecutionReleaser
	segments                           *Segments
	runs                               Projection
	items                              ItemProjection
	admission                          *ownership.Gate
}

// CancellationDependencies are the collaborators Run cancellation requires.
type CancellationDependencies struct {
	RootCancellation                   RunningRootCancellationRequester
	RunningSubtreeCanceler             RunningSubtreeCanceler
	WaitingSubtreeCancellationPreparer WaitingSubtreeCancellationPreparer
	WaitingSubtreeCancellations        WaitingSubtreeCancellationCommitter
	AcceptedSubtreeCancellations       acceptedSubtreeCancellationStore
	SubtreeCancellationRestorer        AcceptedSubtreeCancellationRestorer
	Terminations                       terminationCommitter
	Sessions                           sessionReader
	Interrupts                         pendingInterruptReader
	Checkpoints                        WaitingCheckpointReader
	Releases                           ExecutionReleaser
	Segments                           *Segments
	Runs                               Projection
	Items                              ItemProjection
	Admissions                         *ownership.Gate
}

func NewCancellations(deps CancellationDependencies) (*Cancellations, error) {
	required := []struct {
		name  string
		value any
	}{
		{"root cancellation requester", deps.RootCancellation},
		{"running subtree canceler", deps.RunningSubtreeCanceler},
		{"waiting subtree cancellation preparer", deps.WaitingSubtreeCancellationPreparer},
		{"waiting subtree cancellation committer", deps.WaitingSubtreeCancellations},
		{"accepted subtree cancellation store", deps.AcceptedSubtreeCancellations},
		{"accepted subtree cancellation restorer", deps.SubtreeCancellationRestorer},
		{"run termination committer", deps.Terminations},
		{"session reader", deps.Sessions},
		{"pending interrupt reader", deps.Interrupts},
		{"waiting checkpoint reader", deps.Checkpoints},
		{"execution releaser", deps.Releases},
		{"segment supervisor", deps.Segments},
		{"run projection", deps.Runs},
		{"item projection", deps.Items},
		{"admission gate", deps.Admissions},
	}
	for _, required := range required {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("runs: %s is required", required.name)
		}
	}
	return &Cancellations{
		rootCancellation:                   deps.RootCancellation,
		runningSubtreeCanceler:             deps.RunningSubtreeCanceler,
		waitingSubtreeCancellationPreparer: deps.WaitingSubtreeCancellationPreparer,
		waitingSubtreeCancellations:        deps.WaitingSubtreeCancellations,
		acceptedSubtreeCancellations:       deps.AcceptedSubtreeCancellations,
		subtreeCancellationRestorer:        deps.SubtreeCancellationRestorer,
		terminations:                       deps.Terminations,
		sessionReader:                      deps.Sessions,
		interrupts:                         deps.Interrupts,
		checkpoints:                        deps.Checkpoints,
		releases:                           deps.Releases,
		segments:                           deps.Segments,
		runs:                               deps.Runs,
		items:                              deps.Items,
		admission:                          deps.Admissions,
	}, nil
}

// Cancel handles both live and parked runs under the same run/session admission
// rules and returns the exact terminal Run committed by the winning write-set.
// The durable abandon write-set is authoritative and commits before a parked
// executor is torn down. Cleanup errors are returned unless the executor already
// disappeared, which is the idempotent completion race.
func (c *Cancellations) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	var err error
	cmd, err = cmd.normalizeReason()
	if err != nil {
		return CancelResult{}, err
	}
	plan, entry, live, err := c.cancellationPlanFor(ctx, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	child := plan.target.run.Lineage().IsChild()
	usesLiveOwner := live && (!child || plan.treeState() == rundomain.Running)
	if usesLiveOwner && cmd.Receipt != nil {
		if err := cmd.Receipt.ReserveForRun(ctx, plan.target.run.ID()); err != nil {
			return CancelResult{}, err
		}
	}
	if child {
		switch plan.treeState() {
		case rundomain.Running:
			if !live {
				return CancelResult{}, fmt.Errorf(
					"runs: running child Run %q has no live root owner",
					cmd.RunID,
				)
			}
			return c.cancelLiveChild(cmd, plan, entry.owner)
		case rundomain.Waiting:
			return c.cancelWaitingChild(ctx, cmd, plan)
		default:
			return CancelResult{}, fmt.Errorf(
				"runs: child Run %q belongs to a tree in state %s",
				cmd.RunID,
				plan.treeState(),
			)
		}
	}
	if !live {
		return c.cancelWithoutLiveSegment(ctx, cmd, plan.root.run)
	}
	return c.cancelLiveRoot(cmd, plan, entry.owner)
}

type liveCancellationResult struct {
	result CancelResult
	err    error
}

// The Run task retains the request through control admission and settlement.
// A timeout can end the caller's wait while Scope still owns the control.
func (c *Cancellations) awaitLiveCancellation(
	owner *runTreeOwner,
	receipt CancellationReceipt,
	settle func(context.Context) (CancelResult, error),
) (CancelResult, error) {
	response := make(chan liveCancellationResult, 1)
	if !c.segments.tasks.Start(owner.taskContext, func(ctx context.Context) {
		result, err := settle(ctx)
		response <- liveCancellationResult{result, finishLiveCancellation(ctx, receipt, err)}
	}) {
		return CancelResult{}, finishLiveCancellation(owner.taskContext, receipt, errClosed)
	}
	waitCtx, cancel := owner.cleanupContext()
	defer cancel()
	select {
	case result := <-response:
		return result.result, result.err
	case <-waitCtx.Done():
		return CancelResult{}, &CancellationSettlementError{Cause: context.Cause(waitCtx)}
	}
}

func finishLiveCancellation(ctx context.Context, receipt CancellationReceipt, err error) error {
	if err == nil {
		return nil
	}
	if _, committed := errors.AsType[terminalCancellationRefusal](err); committed {
		return err
	}
	if _, unsettled := errors.AsType[*CancellationSettlementError](err); unsettled {
		return err
	}
	switch {
	case errors.Is(err, ErrRunFinished), errors.Is(err, ErrRunNotFound), errors.Is(err, rundomain.ErrSessionBusy), errors.Is(err, errClosed):
		if receipt != nil {
			if recordErr := receipt.Refuse(ctx, err); recordErr != nil {
				return &CancellationSettlementError{Cause: recordErr}
			}
		}
		return err
	default:
		return &CancellationSettlementError{Cause: err}
	}
}

// A terminal refusal already belongs to the Run's commit. The cleanup waiter
// may observe it after the requester or its process has stopped.
type terminalCancellationRefusal struct{ error }

func (e terminalCancellationRefusal) Unwrap() error { return e.error }

// TerminalCancellationRefusal applies to linked cancellations that have no
// staged result and did not win the terminal transition.
func TerminalCancellationRefusal(value rundomain.Run) error {
	outcome, finished := value.Outcome()
	if !finished {
		return nil
	}
	if loss, lost := value.Loss(); lost && loss.Reason.DeclaredAfterStop() {
		return &CancellationSettlementError{Cause: fmt.Errorf("runs: Run %q was lost", value.ID())}
	}
	return fmt.Errorf("%w: %q settled as %s", ErrRunFinished, value.ID(), outcome)
}

func (t terminalRun) refuseCancellation() error {
	return terminalCancellationRefusal{TerminalCancellationRefusal(t.run)}
}

func (c *Cancellations) cancelLiveChild(
	cmd CancelCommand,
	plan cancellationPlan,
	owner *runTreeOwner,
) (CancelResult, error) {
	return c.awaitLiveCancellation(owner, cmd.Receipt, func(ctx context.Context) (CancelResult, error) {
		return c.settleLiveChildCancellation(ctx, cmd, plan, owner)
	})
}

func (c *Cancellations) settleLiveChildCancellation(
	ctx context.Context,
	cmd CancelCommand,
	plan cancellationPlan,
	owner *runTreeOwner,
) (CancelResult, error) {
	if err := owner.awaitActivation(ctx); err != nil {
		return CancelResult{}, err
	}
	attempt, err := owner.beginChildCancellation(plan, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	requestErr := c.runningSubtreeCanceler.CancelRunningSubtree(
		ctx,
		plan.executor,
		plan.target.memberID,
		attempt.id,
	)
	if errors.Is(requestErr, ErrExecutorParking) {
		if err := owner.wait(ctx); err != nil {
			return CancelResult{}, &CancellationSettlementError{Cause: err}
		}
		if owner.interruptCommitted() {
			return c.cancelWaitingChild(ctx, cmd, plan)
		}
	}
	if requestErr != nil && ctx.Err() == nil && owner.rejectChildCancellation(attempt, requestErr) {
		return CancelResult{}, requestErr
	}
	result, err := owner.waitChildCancellation(ctx, attempt)
	if err != nil && !errors.Is(err, ErrRunFinished) {
		return CancelResult{}, &CancellationSettlementError{Cause: errors.Join(requestErr, err)}
	}
	return result, err
}

// cancelWaitingChild removes one complete child subtree from an executor tree
// parked at a human boundary. It first reserves the Session + working tree,
// claims the execution lifecycle, and prepares one exact executor change that
// keeps its source tree frozen. The application projections and replacement
// executor checkpoint commit in one transaction; only after that durable
// boundary may the prepared change be applied.
//
// When another external boundary survives, the tree stays Waiting. Removing
// the final boundary instead opens continuation Segments in the same transaction
// and immediately continues the already-settled runtime tree—there is no
// synthetic human answer and no transient Running row without an owner.
func (c *Cancellations) cancelWaitingChild(
	ctx context.Context,
	cmd CancelCommand,
	initial cancellationPlan,
) (result CancelResult, err error) {
	// The open interrupt this cancellation settles refuses a relocation for as
	// long as it exists, so the Session read before the claim still names the tree
	// the claim reserves.
	sess, err := c.sessionReader.Get(ctx, initial.root.run.SessionID())
	if err != nil {
		return CancelResult{}, err
	}
	runAdmission, ok, leaseErr := c.admission.AcquireRun(ctx, sess.ID(), sess.Workspace().Path())
	if leaseErr != nil {
		return CancelResult{}, leaseErr
	}
	if !ok {
		return CancelResult{}, fmt.Errorf(
			"%w: session %q or working tree %q has a run or mutation in flight",
			rundomain.ErrSessionBusy,
			sess.ID(),
			sess.Workspace().Path(),
		)
	}
	defer runAdmission.Release()

	cleanupCtx, cancelCleanup := runCleanupContext(ctx)
	defer cancelCleanup()

	plan, err := c.resolveClaimedWaitingChildCancellation(cleanupCtx, cmd)
	if err != nil {
		return CancelResult{}, err
	}

	continuation, err := c.waitingChildCancellationContinuation(cleanupCtx, sess, plan)
	if err != nil {
		return CancelResult{}, err
	}
	request, err := NewWaitingSubtreeCancellationRequest(
		continuation,
		plan.target.memberID,
		cmd.Reason,
	)
	if err != nil {
		return CancelResult{}, fmt.Errorf("runs: prepare waiting child cancellation request: %w", err)
	}
	prepared, err := c.waitingSubtreeCancellationPreparer.PrepareWaitingSubtreeCancellation(
		cleanupCtx,
		request,
	)
	if err != nil {
		return CancelResult{}, err
	}
	defer func() {
		if discardErr := prepared.Discard(); discardErr != nil {
			result = CancelResult{}
			err = errors.Join(
				err,
				fmt.Errorf("runs: discard waiting subtree change: %w", discardErr),
			)
		}
	}()

	transformation, err := prepareWaitingCancellationTransformation(
		plan,
		cmd.Reason,
		c.segments.publications.nowUTC(),
		prepared,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if transformation.remaining != nil {
		return c.commitWaitingChildCancellation(cleanupCtx, plan, transformation, prepared, cmd.Receipt)
	}
	return c.resumeAfterWaitingChildCancellation(
		cleanupCtx,
		sess,
		plan,
		transformation,
		prepared,
		&runAdmission,
		cmd.Receipt,
	)
}

func (c *Cancellations) resolveClaimedWaitingChildCancellation(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlan, error) {
	// The first plan only found the Session to reserve. Resolve it again under
	// admission so Resume or a sibling mutation cannot leave this command acting
	// on stale Pending or Item facts.
	plan, entry, live, err := c.cancellationPlanFor(ctx, cmd)
	if err != nil {
		return cancellationPlan{}, err
	}
	if live {
		if entry.owner == nil {
			return cancellationPlan{}, fmt.Errorf(
				"runs: waiting child Run %q has a live registry entry without an owner",
				cmd.RunID,
			)
		}
		// A just-committed park releases admission immediately before its pump
		// removes the live registry entry. Join that already-terminal boundary and
		// then resolve the now-authoritative parked facts.
		if waitErr := entry.owner.wait(ctx); waitErr != nil {
			return cancellationPlan{}, waitErr
		}
		plan, _, live, err = c.cancellationPlanFor(ctx, cmd)
		if err != nil {
			return cancellationPlan{}, err
		}
		if live {
			return cancellationPlan{}, fmt.Errorf(
				"runs: waiting child Run %q retained a live segment after its parked boundary completed",
				cmd.RunID,
			)
		}
	}
	if plan.treeState() != rundomain.Waiting || !plan.target.run.Lineage().IsChild() {
		return cancellationPlan{}, fmt.Errorf(
			"runs: waiting child cancellation for %q resolved a %s root/child state",
			cmd.RunID,
			plan.treeState(),
		)
	}
	return plan, nil
}

func (c *Cancellations) waitingChildCancellationContinuation(
	ctx context.Context,
	sess session.Session,
	plan cancellationPlan,
) (ExecutionContinuation, error) {
	rootContinuation, ok := plan.pending.RootContinuation()
	if !ok {
		return ExecutionContinuation{}, errors.New("runs: waiting subtree Pending has no root continuation")
	}
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootContinuation.MemberID)
	if err != nil {
		if !errors.Is(err, ErrExecutorStateLost) && !errors.Is(err, ErrExecutorCheckpointNotFound) {
			return ExecutionContinuation{}, err
		}
		lostErr := c.terminations.ApplyRunLost(
			ctx,
			plan.root.run.SessionID(),
			plan.root.run.ID(),
			c.segments.publications.nowUTC(),
		)
		if lostErr != nil {
			return ExecutionContinuation{}, errors.Join(
				err,
				fmt.Errorf("runs: recover lost waiting tree %q: %w", plan.root.run.ID(), lostErr),
			)
		}
		return ExecutionContinuation{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
	}
	return waitingContinuationFromPending(*plan.pending, checkpoint, plan.treeRuns(), sess)
}

func (c *Cancellations) commitWaitingChildCancellation(
	ctx context.Context,
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
	change waitingSubtreeChange,
	receipt CancellationReceipt,
) (CancelResult, error) {
	durable, err := transformation.durableCommit(*plan.pending, newRunCommitID())
	if err != nil {
		return CancelResult{}, err
	}
	result, err := c.waitingSubtreeCancellations.CommitWaitingSubtreeCancellation(
		ctx,
		durable,
		receipt,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if err := change.Apply(WaitingSubtreeStaysWaiting); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	if err := c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, plan.root.run.ID(), plan.target.run.ID()); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	c.publishWaitingChildCancellation(plan, transformation)
	return CancelResult{Run: result.TargetRun, RootRun: &result.RootRun}, nil
}

func (c *Cancellations) resumeAfterWaitingChildCancellation(
	ctx context.Context,
	sess session.Session,
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
	change waitingSubtreeChange,
	runAdmission *ownership.RunAdmission,
	receipt CancellationReceipt,
) (CancelResult, error) {
	segmentID := c.segments.newSegmentID()
	var committed WaitingSubtreeCancellationResult
	_, err := c.segments.openSegment(ctx, segmentSpec{
		RunID:        plan.root.run.ID(),
		SegmentID:    segmentID,
		SessionID:    plan.root.run.SessionID(),
		WorkspaceCWD: sess.Workspace().Path(),
		Isolated:     sess.Isolated(),
		ExecutorID:   plan.executor.ExecutorID,
		admission:    runAdmission,
		Opening: treeResumption{Continuation: transformation.continuation, CommitOpening: func(commitCtx context.Context, opening OpeningCommit) error {
			_, admitting := opening.Admission()
			resume, resuming := opening.Resume()
			if admitting || !resuming {
				return errors.New("runs: waiting child continuation produced an invalid opening disposition")
			}
			durable, durableErr := transformation.resumedDurableCommit(
				*plan.pending, opening.CommitID(), resume, opening.Events(),
			)
			if durableErr != nil {
				return durableErr
			}
			result, commitErr := c.waitingSubtreeCancellations.CommitWaitingSubtreeCancellation(commitCtx, durable, receipt)
			if commitErr == nil {
				committed = result
			}
			return commitErr
		}},
		BeginExecution: func(beginCtx context.Context) error {
			if err := change.Apply(WaitingSubtreeResumesRunning); err != nil {
				return fmt.Errorf(
					"runs: apply committed cancellation of waiting child Run %q in root Run %q: %w",
					plan.target.run.ID(),
					plan.root.run.ID(),
					err,
				)
			}
			if err := change.Continue(beginCtx); err != nil {
				return fmt.Errorf(
					"runs: continue root Run %q after canceling final waiting child Run %q: %w",
					plan.root.run.ID(),
					plan.target.run.ID(),
					err,
				)
			}
			return nil
		},
	})
	if err != nil {
		if committed.TargetRun.ID() != "" {
			return CancelResult{}, &CancellationSettlementError{Cause: err}
		}
		return CancelResult{}, err
	}
	if err := c.acceptedSubtreeCancellations.DeleteSubtreeCancellation(ctx, plan.root.run.ID(), plan.target.run.ID()); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	c.publishWaitingChildCancellation(plan, transformation)
	return CancelResult{Run: committed.TargetRun, RootRun: &committed.RootRun}, nil
}

func (c *Cancellations) publishWaitingChildCancellation(
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
) {
	affected := make([]string, 0, len(transformation.canceledRunIDs)+len(plan.survivingTree)+2)
	seen := make(map[string]struct{}, cap(affected))
	appendRunID := func(runID string) {
		if runID == "" {
			return
		}
		if _, duplicate := seen[runID]; duplicate {
			return
		}
		seen[runID] = struct{}{}
		affected = append(affected, runID)
	}
	for _, runID := range transformation.canceledRunIDs {
		appendRunID(runID)
	}
	appendRunID(plan.target.run.Lineage().ParentRunID())
	if transformation.remaining == nil {
		for _, continuation := range transformation.continuation.continuations {
			appendRunID(continuation.RunID)
		}
	}
	appendRunID(plan.root.run.ID())
	c.segments.publications.publishWaitingSubtreeCanceled(
		plan.root.run.SessionID(),
		plan.root.run.ID(),
		affected,
	)
}

func rootCancelResult(value rundomain.Run) (CancelResult, error) {
	if value.Lineage().IsChild() {
		return CancelResult{}, fmt.Errorf("runs: canceled root result %q is a child Run", value.ID())
	}
	if !value.Canceled() {
		return CancelResult{}, fmt.Errorf("runs: cancel committed invalid terminal Run %q in state %s", value.ID(), value.State())
	}
	return CancelResult{Run: value}, nil
}
