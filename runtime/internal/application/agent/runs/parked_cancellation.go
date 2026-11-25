package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Claiming the Session before resolving the open interrupt linearizes cancel
// against resume: whichever command owns the Session decides the one durable
// transition.
func (c *Cancellations) cancelParkedRoot(ctx context.Context, cmd CancelCommand, sessionID identity.SessionID) (CancelResult, error) {
	releaseSession, ok, leaseErr := c.admission.AcquireSession(ctx, sessionID)
	if leaseErr != nil {
		return CancelResult{}, leaseErr
	}
	if !ok {
		return CancelResult{}, rundomain.ErrSessionBusy
	}
	defer releaseSession()

	pending, found, err := c.interrupts.Get(ctx, cmd.RunID)
	if err != nil {
		return CancelResult{}, err
	}
	if !found {
		refreshed, exists, lookupErr := c.runs.Run(ctx, cmd.RunID)
		switch {
		case lookupErr != nil:
			return CancelResult{}, lookupErr
		case !exists:
			return CancelResult{}, fmt.Errorf("%w: %q", ErrRunNotFound, cmd.RunID)
		case refreshed.State().IsTerminal():
			return CancelResult{}, terminalCancellationRefusal{TerminalCancellationRefusal(refreshed)}
		default:
			return CancelResult{}, fmt.Errorf("runs: run %q is %s but has no open interrupt", cmd.RunID, refreshed.State())
		}
	}
	terminal, err := c.terminations.CommitParkedTermination(
		ctx, sessionID, pending, c.segments.publications.nowUTC(), rundomain.Canceled{Detail: cmd.Reason}, cmd.Receipt,
	)
	if err != nil {
		return CancelResult{}, err
	}
	ref := ExecutorRef{SessionID: sessionID, ExecutorID: pending.ExecutorID}
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
