package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

var (
	ErrCheckpointUnavailable       = errors.New("sessions: checkpoint unavailable")
	ErrCheckpointConflict          = errors.New("sessions: checkpoint restore conflicts with unarchived material")
	ErrRollbackRecoveryPending     = errors.New("sessions: file rollback recovery is pending")
	ErrCheckpointRestoreIncomplete = errors.New("sessions: checkpoint restore may be incomplete")
)

const mutationCleanupTimeout = 5 * time.Second

// RollbackReceiptEncoder renders a file rollback's stored reply. Recovery
// settles a retained intent without its original request, so the encoding
// depends only on the outcome.
type RollbackReceiptEncoder func(sessionID identity.SessionID, result RollbackResult, cause error) ([]byte, error)

// rollbackAttempt says whether this attempt recorded the intent. Only the
// recording attempt can prove no checkout has started, so only it may settle a
// refusal and release the intent.
type rollbackAttempt uint8

const (
	recordingAttempt rollbackAttempt = iota + 1
	resumedAttempt
)

func (c *Coordinator) RollbackWorkspace(ctx context.Context, spec WorkspaceRollbackSpec) (RollbackResult, error) {
	if spec.ToRunID.IsZero() {
		return RollbackResult{}, errors.New("sessions: file restore requires target Run")
	}
	release, claimed, err := c.admissions.AcquireWorkspaceRollback(ctx, spec.SessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	if !claimed {
		return RollbackResult{}, run.ErrSessionBusy
	}
	defer release()

	current, err := c.Get(ctx, spec.SessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	cwd := current.Workspace().Path()
	tree, claimed, err := c.ClaimWorkingTreeMutation(cwd)
	if err != nil {
		return RollbackResult{}, err
	}
	if !claimed {
		return RollbackResult{}, fmt.Errorf("%w: working tree %q has a Run in flight", run.ErrSessionBusy, cwd)
	}
	defer tree.Release()

	boundary, err := c.resolveWorkspaceRollbackBoundary(ctx, spec)
	if err != nil {
		return RollbackResult{}, err
	}
	if surviving, found := boundary.survivingContinuation(); found {
		return RollbackResult{}, fmt.Errorf(
			"%w: Run %q is %s and this boundary keeps it; settle it before restoring files under it",
			run.ErrSessionBusy, surviving.ID(), surviving.State(),
		)
	}
	mutation := WorkspaceMutation{WorkspaceRollbackSpec: spec, CWD: cwd}
	recorded, err := c.mutations.Record(ctx, mutation)
	if err != nil {
		return RollbackResult{}, err
	}
	attempt := resumedAttempt
	if recorded {
		attempt = recordingAttempt
	}
	return c.restoreWorkspace(ctx, mutation, boundary, attempt)
}

func (c *Coordinator) resolveWorkspaceRollbackBoundary(ctx context.Context, spec WorkspaceRollbackSpec) (resolvedRollbackBoundary, error) {
	boundary, err := c.resolveRollbackBoundary(ctx, spec.SessionID, spec.ToRunID)
	if err != nil || spec.RestoreHistory {
		return boundary, err
	}
	return boundary.keepingHistory(), nil
}

// restoreWorkspace always renders the reply; the intent's store keeps it only
// when the request carried an idempotency key.
func (c *Coordinator) restoreWorkspace(ctx context.Context, m WorkspaceMutation, boundary resolvedRollbackBoundary, attempt rollbackAttempt) (RollbackResult, error) {
	if err := c.quiesceRollbackWorkspace(m.SessionID, m.CWD); err != nil {
		return RollbackResult{}, c.refuseWorkspaceRollback(ctx, m, attempt, err)
	}
	if err := c.checkpoints.Restore(ctx, m.SessionID, m.CWD, m.ToRunID); err != nil {
		if errors.Is(err, ErrCheckpointRestoreIncomplete) {
			c.transientState.ForgetWorkspace(m.CWD)
			return RollbackResult{}, errors.Join(ErrRollbackRecoveryPending, err)
		}
		return RollbackResult{}, c.refuseWorkspaceRollback(ctx, m, attempt, err)
	}
	c.transientState.ForgetWorkspace(m.CWD)
	if boundary.keepsHistory() {
		if err := c.sandbox.Discard(m.SessionID); err != nil {
			return RollbackResult{}, errors.Join(ErrRollbackRecoveryPending, err)
		}
	}
	var result RollbackResult
	err := c.completeRollback(ctx, m.SessionID, boundary, func(ctx context.Context, write func(context.Context) (RollbackResult, error)) error {
		return c.mutations.Complete(ctx, m, func(tx context.Context) ([]byte, error) {
			var err error
			result, err = write(tx)
			if err != nil {
				return nil, err
			}
			return c.rollbackReceipts(m.SessionID, result, nil)
		})
	})
	if err != nil {
		return RollbackResult{}, errors.Join(ErrRollbackRecoveryPending, err)
	}
	return result, nil
}

func (c *Coordinator) quiesceRollbackWorkspace(sessionID identity.SessionID, cwd string) error {
	// Session shells can live in an isolated copy. Other Sessions can own
	// shells in the shared tree, so both scopes must stop before checkout.
	if err := c.transientState.QuiesceSession(sessionID); err != nil {
		return fmt.Errorf("sessions: quiesce process-local Session state before file rollback: %w", err)
	}
	if err := c.transientState.QuiesceWorkspace(cwd); err != nil {
		return fmt.Errorf("sessions: quiesce working tree before file rollback: %w", err)
	}
	return nil
}

func (c *Coordinator) refuseWorkspaceRollback(ctx context.Context, m WorkspaceMutation, attempt rollbackAttempt, cause error) error {
	if attempt != recordingAttempt {
		return errors.Join(ErrRollbackRecoveryPending, cause)
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationCleanupTimeout)
	defer cancel()
	err := c.mutations.Complete(cleanup, m, func(context.Context) ([]byte, error) {
		return c.rollbackReceipts(m.SessionID, RollbackResult{}, cause)
	})
	if err != nil {
		return errors.Join(ErrRollbackRecoveryPending, cause, err)
	}
	return cause
}

// Recovery runs before any new command can take the Session or tree. The
// retained intent alone authorizes another checkout of this exact boundary.
func (c *Coordinator) RecoverWorkspaceMutations(ctx context.Context) error {
	pending, err := c.mutations.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := c.recoverWorkspaceRollback(ctx, m); err != nil {
			return fmt.Errorf("sessions: recover rollback for %q: %w", m.SessionID, err)
		}
	}
	return nil
}

func (c *Coordinator) recoverWorkspaceRollback(ctx context.Context, m WorkspaceMutation) error {
	current, err := c.Get(ctx, m.SessionID)
	if err != nil {
		return err
	}
	if current.Workspace().Path() != m.CWD {
		return errors.New("sessions: retained rollback has a different working tree")
	}
	boundary, err := c.resolveWorkspaceRollbackBoundary(ctx, m.WorkspaceRollbackSpec)
	if err != nil {
		return err
	}
	_, err = c.restoreWorkspace(ctx, m, boundary, resumedAttempt)
	return err
}
