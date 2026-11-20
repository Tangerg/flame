package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// cancelWithoutLiveSegment resolves the small window in which a segment has
// left the live registry after the first durable read. A second durable read
// classifies a real terminal race; a still-running orphan is an invariant fault,
// never run_not_found.
func (c *Cancellations) cancelWithoutLiveSegment(ctx context.Context, cmd CancelCommand, value rundomain.Run) (CancelResult, error) {
	if value.State() == rundomain.Waiting {
		cleanupCtx, cancel := runCleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, value)
	}
	refreshed, found, err := c.runs.Run(ctx, cmd.RunID)
	switch {
	case err != nil:
		return CancelResult{}, err
	case !found:
		return CancelResult{}, fmt.Errorf("runs: run %q disappeared after it was resolved", cmd.RunID)
	case refreshed.State().IsTerminal():
		return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State())
	case refreshed.State() == rundomain.Waiting:
		cleanupCtx, cancel := runCleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, refreshed)
	case refreshed.State() == rundomain.Running:
		return CancelResult{}, fmt.Errorf(
			"runs: run %q is running segment %q with no live owner",
			cmd.RunID, refreshed.ActiveSegmentID(),
		)
	default:
		return CancelResult{}, fmt.Errorf("runs: run %q has unknown state %q", cmd.RunID, refreshed.State())
	}
}

// cancelParkedRun claims the Session before resolving its open interrupt. That
// admission order linearizes cancel against resume: whichever command owns the
// Session decides the one durable transition, and the loser observes busy or
// the resulting terminal state instead of misreporting run_not_found.
func (c *Cancellations) cancelParkedRun(ctx context.Context, cmd CancelCommand, value rundomain.Run) (CancelResult, error) {
	releaseSession, ok, leaseErr := c.admission.AcquireSession(ctx, value.SessionID())
	if leaseErr != nil {
		return CancelResult{}, leaseErr
	}
	if !ok {
		return CancelResult{}, rundomain.ErrSessionBusy
	}
	defer releaseSession()

	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return CancelResult{}, err
	}
	if !found {
		refreshed, exists, lookupErr := c.runs.Run(ctx, cmd.RunID)
		switch {
		case lookupErr != nil:
			return CancelResult{}, lookupErr
		case !exists:
			return CancelResult{}, fmt.Errorf("runs: parked run %q disappeared while its session was claimed", cmd.RunID)
		case refreshed.State().IsTerminal():
			return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State())
		default:
			return CancelResult{}, fmt.Errorf("runs: run %q is %s but has no open interrupt", cmd.RunID, refreshed.State())
		}
	}
	return c.cancelClaimedParkedRun(ctx, cmd, ExecutorRef{
		SessionID:  value.SessionID(),
		ExecutorID: pending.ExecutorID,
	})
}

// cancelKnownParkedRun is used only after the live Run-tree owner proves its
// interrupt transaction committed. The owner's Segment binding is therefore the exact
// executor that transaction parked; resolving it again would introduce a second,
// weaker source of identity between two halves of one command.
func (c *Cancellations) cancelKnownParkedRun(
	ctx context.Context,
	cmd CancelCommand,
	value rundomain.Run,
	ref ExecutorRef,
) (CancelResult, error) {
	if ref.SessionID != value.SessionID() {
		return CancelResult{}, fmt.Errorf(
			"runs: Run %q belongs to Session %q but its live executor belongs to %q",
			cmd.RunID, value.SessionID(), ref.SessionID,
		)
	}
	releaseSession, ok, leaseErr := c.admission.AcquireSession(ctx, value.SessionID())
	if leaseErr != nil {
		return CancelResult{}, leaseErr
	}
	if !ok {
		return CancelResult{}, rundomain.ErrSessionBusy
	}
	defer releaseSession()
	return c.cancelClaimedParkedRun(ctx, cmd, ref)
}

func (c *Cancellations) cancelClaimedParkedRun(ctx context.Context, cmd CancelCommand, ref ExecutorRef) (CancelResult, error) {
	terminal, err := c.terminations.ApplyRunCancel(ctx, ref, cmd.RunID, cmd.Reason, c.segments.publications.nowUTC(), cmd.Receipt)
	if err != nil {
		return CancelResult{}, err
	}
	if err := c.SettleRootCancellation(ctx, ref, cmd.RunID); err != nil {
		return CancelResult{}, err
	}
	return rootCancelResult(terminal)
}

func (c *Cancellations) SettleRootCancellation(ctx context.Context, ref ExecutorRef, runID identity.RunID) error {
	if ref.SessionID.IsZero() || ref.ExecutorID == "" || runID.IsZero() {
		return ErrInvalidExecutorRef
	}
	cleanupCtx, cancel := runCleanupContext(ctx)
	defer cancel()
	if live, found := c.segments.registry.Get(runID); found {
		if live.record.ExecutorID != ref.ExecutorID {
			return ErrInvalidExecutorRef
		}
		if err := live.owner.wait(cleanupCtx); err != nil {
			return &CancellationSettlementError{Cause: err}
		}
	}
	if err := c.releases.Release(cleanupCtx, ref); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		return &CancellationSettlementError{Cause: fmt.Errorf("release canceled executor %q: %w", ref.ExecutorID, err)}
	}
	return nil
}
