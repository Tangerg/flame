package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/samber/lo"
)

type CancellationSettlementError struct{ Cause error }

func (e *CancellationSettlementError) Error() string {
	if e.Cause == nil {
		return "runs: cancellation settlement failed"
	}
	return fmt.Sprintf("runs: cancellation settlement failed: %v", e.Cause)
}

func (e *CancellationSettlementError) Unwrap() error { return e.Cause }

type CancellationTarget struct {
	Run      rundomain.Run
	Root     rundomain.Run
	MemberID string
}

type cancellationProjection interface {
	CancellationTarget(context.Context, identity.RunID) (CancellationTarget, bool, error)
	Run(context.Context, identity.RunID) (rundomain.Run, bool, error)
	Tree(context.Context, identity.RunID) ([]rundomain.Run, error)
}

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
	runs                               cancellationProjection
	items                              ItemProjection
	admission                          *ownership.Gate
}

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
	Runs                               cancellationProjection
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

func (c *Cancellations) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	var err error
	cmd, err = cmd.normalizeReason()
	if err != nil {
		return CancelResult{}, err
	}
	target, found, err := c.runs.CancellationTarget(ctx, cmd.RunID)
	if err != nil {
		return CancelResult{}, err
	}
	if !found {
		return CancelResult{}, fmt.Errorf("%w: %q", ErrRunNotFound, cmd.RunID)
	}
	child := target.Run.Lineage().IsChild()
	if child && !cmd.AllowChildRun {
		return CancelResult{}, fmt.Errorf("%w: %q", ErrChildRunNotAllowed, cmd.RunID)
	}
	if target.Run.State().IsTerminal() {
		return CancelResult{}, fmt.Errorf("%w: %q", ErrRunFinished, cmd.RunID)
	}
	if target.Run.State() != target.Root.State() {
		return CancelResult{}, fmt.Errorf(
			"runs: Run %q is %s while root Run %q is %s",
			cmd.RunID, target.Run.State(), target.Root.ID(), target.Root.State(),
		)
	}
	switch target.Root.State() {
	case rundomain.Waiting:
		if child {
			return c.cancelWaitingChild(ctx, cmd, target.Root.SessionID())
		}
		cleanup, cancel := runCleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanup, cmd, target.Root)
	case rundomain.Running:
	default:
		return CancelResult{}, fmt.Errorf(
			"runs: open Run %q belongs to root %q in state %s",
			cmd.RunID, target.Root.ID(), target.Root.State(),
		)
	}
	if child && target.MemberID == "" {
		return CancelResult{}, fmt.Errorf("runs: running child Run %q has no admitted executor member", cmd.RunID)
	}
	live, err := c.liveCancellationSegment(ctx, target)
	if err != nil {
		return CancelResult{}, err
	}
	if cmd.Receipt != nil {
		if err := cmd.Receipt.ReserveForRun(ctx, cmd.RunID); err != nil {
			return CancelResult{}, err
		}
	}
	ref := ExecutorRef{SessionID: target.Root.SessionID(), ExecutorID: live.record.ExecutorID}
	if child {
		return c.cancelLiveChild(cmd, target, ref, live.owner)
	}
	return c.cancelLiveRoot(cmd, target.Root, ref, live.owner)
}

func (c *Cancellations) liveCancellationSegment(ctx context.Context, target CancellationTarget) (liveSegment, error) {
	live, found := c.segments.registry.Running(target.Root.ID())
	if found && target.Root.Executes(live.record.SegmentID) {
		return live, nil
	}
	// A retiring owner can leave the registry after the durable snapshot.
	current, exists, err := c.runs.Run(ctx, target.Run.ID())
	switch {
	case err != nil:
		return liveSegment{}, err
	case !exists:
		return liveSegment{}, fmt.Errorf("%w: %q", ErrRunNotFound, target.Run.ID())
	case current.State().IsTerminal():
		return liveSegment{}, fmt.Errorf("%w: %q", ErrRunFinished, current.ID())
	case !current.Executes(target.Run.ActiveSegmentID()):
		return liveSegment{}, fmt.Errorf("%w: Run %q changed segments during cancellation", rundomain.ErrSessionBusy, current.ID())
	default:
		return liveSegment{}, fmt.Errorf("runs: root Run %q has no live owner for segment %q", target.Root.ID(), target.Root.ActiveSegmentID())
	}
}

type liveCancellationResult struct {
	result CancelResult
	err    error
}

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

type terminalCancellationRefusal struct{ error }

func (e terminalCancellationRefusal) Unwrap() error { return e.error }

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
	target CancellationTarget,
	ref ExecutorRef,
	owner *runTreeOwner,
) (CancelResult, error) {
	return c.awaitLiveCancellation(owner, cmd.Receipt, func(ctx context.Context) (CancelResult, error) {
		return c.settleLiveChildCancellation(ctx, cmd, target, ref, owner)
	})
}

func (c *Cancellations) settleLiveChildCancellation(
	ctx context.Context,
	cmd CancelCommand,
	target CancellationTarget,
	ref ExecutorRef,
	owner *runTreeOwner,
) (CancelResult, error) {
	if err := owner.awaitActivation(ctx); err != nil {
		return CancelResult{}, err
	}
	attempt, err := owner.beginChildCancellation(target, ref, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	requestErr := c.runningSubtreeCanceler.CancelRunningSubtree(
		ctx,
		ref,
		target.MemberID,
		attempt.id,
	)
	if errors.Is(requestErr, ErrExecutorParking) {
		if err := owner.wait(ctx); err != nil {
			return CancelResult{}, &CancellationSettlementError{Cause: err}
		}
		if owner.interruptCommitted() {
			return c.cancelWaitingChild(ctx, cmd, ref.SessionID)
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

func rootCancelResult(value rundomain.Run) (CancelResult, error) {
	if value.Lineage().IsChild() {
		return CancelResult{}, fmt.Errorf("runs: canceled root result %q is a child Run", value.ID())
	}
	if !value.Canceled() {
		return CancelResult{}, fmt.Errorf("runs: cancel committed invalid terminal Run %q in state %s", value.ID(), value.State())
	}
	return CancelResult{Run: value}, nil
}
