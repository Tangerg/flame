package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/completion"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/google/uuid"
)

// RootCancellationID correlates an executor control with one accepted request.
// The caller's reason is user data and can be shared by different requests.
type RootCancellationID uuid.UUID

func (i RootCancellationID) String() string { return uuid.UUID(i).String() }
func (i RootCancellationID) IsZero() bool   { return i == RootCancellationID{} }

func ParseRootCancellationID(value string) (RootCancellationID, bool) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		return RootCancellationID{}, false
	}
	return RootCancellationID(id), true
}

type rootCancellation struct {
	id     RootCancellationID
	reason string
	record RootCancellationReceipt
}

func (r *runTreeOwner) matchingRootCancellation(id RootCancellationID) *rootCancellation {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rootCancel == nil || r.rootCancel.id != id {
		return nil
	}
	return r.rootCancel
}

func (r *runTreeOwner) suppressedActivationCancellation() *rootCancellation {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activation.phase != activationSuppressed {
		return nil
	}
	return r.rootCancel
}

type rootCancellationClaim struct {
	interruptDone <-chan struct{}
	dispatch      bool
}

type rootCancellationResult struct {
	result CancelResult
	err    error
}

// The owned task retains its original receipt through control admission and
// terminal settlement, even after the waiting request reaches its deadline.
func (c *Cancellations) cancelLiveRoot(cmd CancelCommand, plan cancellationPlan, owner *runTreeOwner) (CancelResult, error) {
	intent := &rootCancellation{
		id: RootCancellationID(uuid.New()), reason: cmd.Reason, record: cmd.Completion.RecordActiveRoot,
	}
	response := make(chan rootCancellationResult, 1)
	if !c.segments.tasks.Start(owner.taskContext, func(ctx context.Context) {
		result, err := c.settleLiveRootCancellation(ctx, cmd, plan, owner, intent)
		response <- rootCancellationResult{result, err}
	}) {
		return CancelResult{}, errClosed
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

func (c *Cancellations) settleLiveRootCancellation(
	ctx context.Context,
	cmd CancelCommand,
	plan cancellationPlan,
	owner *runTreeOwner,
	intent *rootCancellation,
) (CancelResult, error) {
	claim, err := owner.claimRootCancellation(ctx, intent)
	if err != nil {
		return CancelResult{}, err
	}
	if err := completion.Wait(ctx, claim.interruptDone); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: err}
	}
	if owner.interruptCommitted() {
		if err := owner.wait(ctx); err != nil {
			return CancelResult{}, &CancellationSettlementError{Cause: err}
		}
		return c.cancelKnownParkedRun(ctx, cmd, plan.root.run, plan.executor)
	}
	var requestErr error
	if claim.dispatch {
		requestErr = c.rootCancellation.RequestRootCancellation(ctx, plan.executor, intent.id)
	}
	if err := owner.wait(ctx); err != nil {
		return CancelResult{}, &CancellationSettlementError{Cause: errors.Join(requestErr, err)}
	}
	if terminal, committed := owner.committedRootTerminal(); committed {
		if terminal.cancellation == intent.id {
			return rootCancelResult(terminal.run)
		}
		if requestErr == nil || errors.Is(requestErr, ErrExecutorNotLive) {
			ending, _ := terminal.run.Ending()
			return CancelResult{}, fmt.Errorf("%w: %q settled as %s", ErrRunFinished, cmd.RunID, ending.Outcome())
		}
	}
	if requestErr != nil {
		return CancelResult{}, requestErr
	}
	return CancelResult{}, fmt.Errorf("runs: canceled live root Run %q completed without a terminal snapshot", cmd.RunID)
}

func (r *runTreeOwner) claimRootCancellation(ctx context.Context, intent *rootCancellation) (rootCancellationClaim, error) {
	for {
		r.mu.Lock()
		if r.childCancel != nil {
			targetRunID := r.childCancel.targetRunID
			r.mu.Unlock()
			return rootCancellationClaim{}, fmt.Errorf(
				"%w: child Run %q cancellation owns the tree",
				rundomain.ErrSessionBusy,
				targetRunID,
			)
		}
		if r.rootCancel != nil {
			r.mu.Unlock()
			return rootCancellationClaim{}, fmt.Errorf(
				"%w: root Run cancellation already owns the tree",
				rundomain.ErrSessionBusy,
			)
		}
		activation := r.activation
		if activation.phase == activationStarted {
			r.mu.Unlock()
			select {
			case <-activation.done:
				continue
			case <-ctx.Done():
				return rootCancellationClaim{}, context.Cause(ctx)
			}
		}
		if activation.err != nil {
			r.mu.Unlock()
			return rootCancellationClaim{}, fmt.Errorf("%w: segment activation failed: %w", ErrRunFinished, activation.err)
		}
		r.rootCancel = intent
		claim := rootCancellationClaim{interruptDone: r.interrupt.active, dispatch: activation.phase != activationPending}
		r.mu.Unlock()
		return claim, nil
	}
}

func (r *runTreeOwner) interruptCommitted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.interrupt.committed
}
