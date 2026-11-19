package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

var (
	ErrCheckpointUnavailable       = errors.New("sessions: checkpoint unavailable")
	ErrCheckpointConflict          = errors.New("sessions: checkpoint restore conflicts with unarchived material")
	ErrRollbackRecoveryPending     = errors.New("sessions: file rollback recovery is pending")
	ErrCheckpointRestoreIncomplete = errors.New("sessions: checkpoint restore may be incomplete")
)

const mutationCleanupTimeout = 5 * time.Second

type RollbackReceiptEncoder func(sessionID string, result RollbackResult, cause error) ([]byte, error)

func (c *Coordinator) RollbackWorkspace(ctx context.Context, spec WorkspaceRollbackSpec, encode RollbackReceiptEncoder) (RollbackResult, error) {
	if spec.ToRunID == "" {
		return RollbackResult{}, errors.New("sessions: file restore requires target Run")
	}
	if spec.Receipt.Key != "" && encode == nil {
		return RollbackResult{}, errors.New("sessions: file rollback receipt encoder is required")
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

	boundary, err := c.resolveRollbackBoundary(ctx, spec.SessionID, spec.ToRunID)
	if err != nil {
		return RollbackResult{}, err
	}
	if surviving, found := boundary.survivingContinuation(spec.RestoreHistory); found {
		return RollbackResult{}, fmt.Errorf(
			"%w: Run %q is %s and this boundary keeps it; settle it before restoring files under it",
			run.ErrSessionBusy, surviving.ID(), surviving.State(),
		)
	}
	mutation := WorkspaceMutation{WorkspaceRollbackSpec: spec, CWD: cwd}
	fresh, err := c.mutations.Record(ctx, mutation)
	if err != nil {
		return RollbackResult{}, err
	}
	return c.restoreWorkspace(ctx, mutation, boundary, fresh, encode)
}

func (c *Coordinator) restoreWorkspace(ctx context.Context, m WorkspaceMutation, boundary resolvedRollbackBoundary, fresh bool, encode RollbackReceiptEncoder) (RollbackResult, error) {
	if err := c.quiesceRollbackWorkspace(m.SessionID, m.CWD); err != nil {
		return RollbackResult{}, c.refuseWorkspaceRollback(ctx, m, fresh, encode, err)
	}
	if err := c.checkpoints.Restore(ctx, m.SessionID, m.CWD, m.ToRunID); err != nil {
		if errors.Is(err, ErrCheckpointRestoreIncomplete) {
			c.transientState.ForgetWorkspace(m.CWD)
			return RollbackResult{}, errors.Join(ErrRollbackRecoveryPending, err)
		}
		return RollbackResult{}, c.refuseWorkspaceRollback(ctx, m, fresh, encode, err)
	}
	c.transientState.ForgetWorkspace(m.CWD)
	if !m.RestoreHistory || len(boundary.timeline.Dropped) == 0 {
		if err := c.sandbox.Discard(m.SessionID); err != nil {
			return RollbackResult{}, errors.Join(ErrRollbackRecoveryPending, err)
		}
	}
	var result RollbackResult
	err := c.completeRollback(ctx, m.SessionID, boundary, m.RestoreHistory, func(ctx context.Context, write func(context.Context) (RollbackResult, error)) error {
		return c.mutations.Complete(ctx, m, func(tx context.Context) ([]byte, error) {
			var err error
			result, err = write(tx)
			if err != nil || m.Receipt.Key == "" {
				return nil, err
			}
			return encode(m.SessionID, result, nil)
		})
	})
	if err != nil {
		return RollbackResult{}, errors.Join(ErrRollbackRecoveryPending, err)
	}
	return result, nil
}

func (c *Coordinator) quiesceRollbackWorkspace(sessionID, cwd string) error {
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

func (c *Coordinator) refuseWorkspaceRollback(ctx context.Context, m WorkspaceMutation, fresh bool, encode RollbackReceiptEncoder, cause error) error {
	// Only the first attempt can prove no checkout has started.
	if !fresh {
		return errors.Join(ErrRollbackRecoveryPending, cause)
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationCleanupTimeout)
	defer cancel()
	err := c.mutations.Complete(cleanup, m, func(context.Context) ([]byte, error) {
		if m.Receipt.Key == "" {
			return nil, nil
		}
		return encode(m.SessionID, RollbackResult{}, cause)
	})
	if err != nil {
		return errors.Join(ErrRollbackRecoveryPending, cause, err)
	}
	return cause
}

// Recovery runs before any new command can take the Session or tree. The
// retained intent alone authorizes another checkout of this exact boundary.
func (c *Coordinator) RecoverWorkspaceMutations(ctx context.Context, encode RollbackReceiptEncoder) error {
	pending, err := c.mutations.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := c.recoverWorkspaceRollback(ctx, m, encode); err != nil {
			return fmt.Errorf("sessions: recover rollback for %q: %w", m.SessionID, err)
		}
	}
	return nil
}

func (c *Coordinator) recoverWorkspaceRollback(ctx context.Context, m WorkspaceMutation, encode RollbackReceiptEncoder) error {
	current, err := c.Get(ctx, m.SessionID)
	if err != nil {
		return err
	}
	if current.Workspace().Path() != m.CWD {
		return errors.New("sessions: retained rollback has a different working tree")
	}
	boundary, err := c.resolveRollbackBoundary(ctx, m.SessionID, m.ToRunID)
	if err != nil {
		return err
	}
	_, err = c.restoreWorkspace(ctx, m, boundary, false, encode)
	return err
}
