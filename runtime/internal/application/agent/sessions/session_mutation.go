package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// DeleteSession first quiesces detached processes while the Session mutation
// admission is held, atomically removes all durable state, then tears down
// parked executions and non-failing process-local markers. The open interrupts
// are read up front so abandoned executions can be canceled after the durable
// state is gone. Checkpoint and sandbox cleanup run last; all post-commit
// cleanup failures are returned together.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	admission, err := c.ClaimSessionMutation(sessionID)
	if err != nil {
		return err
	}
	defer admission.Release()

	var pending []runs.Pending
	return c.withGoalMutation(
		ctx,
		[]string{sessionID},
		func(commitCtx context.Context) error {
			open, err := c.interrupts.List(commitCtx, sessionID)
			if err != nil {
				return err
			}
			pending = append(pending, open...)
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before delete: %w", err)
			}
			return c.writes.ApplyDelete(commitCtx, DeletePlan{SessionID: sessionID})
		},
		func(ctx context.Context) error {
			// The durable cascade is gone as of here, so the signal cannot outrun it —
			// and it goes out before the process-local cleanup, whose failures are the
			// caller's to report but change nothing a client can read.
			c.publishAggregateMoved([]string{sessionID}, nil)
			var cleanupErrs []error
			for _, item := range pending {
				if err := c.releaseExecution(ctx, RunExecutionBinding{
					RunID:      item.RootRunID,
					SessionID:  item.SessionID,
					ExecutorID: item.ExecutorID,
				}); err != nil {
					cleanupErrs = append(cleanupErrs, err)
				}
			}
			cleanupErrs = append(cleanupErrs, c.dropSessionResources([]string{sessionID}, "deleted")...)
			return errors.Join(cleanupErrs...)
		},
	)
}

// dropSessionResources removes process-local resources after a durable Session
// delete. The action preserves useful error context for the operator.
func (c *Coordinator) dropSessionResources(sessionIDs []string, action string) []error {
	var errs []error
	for _, sessionID := range sessionIDs {
		c.transientState.ForgetSession(sessionID)
		if c.checkpoints != nil {
			if err := c.checkpoints.DropSession(sessionID); err != nil {
				errs = append(errs, fmt.Errorf("sessions: drop checkpoints for %s session %q: %w", action, sessionID, err))
			}
		}
		if c.sandbox != nil {
			if err := c.sandbox.Discard(sessionID); err != nil {
				errs = append(errs, fmt.Errorf("sessions: discard sandbox copy for %s session %q: %w", action, sessionID, err))
			}
		}
	}
	return errs
}

func (c *Coordinator) withGoalMutation(
	ctx context.Context,
	sessionIDs []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	if c.goals == nil {
		if err := commit(ctx); err != nil {
			return err
		}
		return afterCommit(ctx)
	}
	return c.goals.WithSessionMutation(ctx, sessionIDs, commit, afterCommit)
}

// restoreSession applies a canonical archive and, when requested, derives its
// session view before releasing the mutation admission. A restoration must not
// expose a separately-read view because another mutation could otherwise
// interleave between the durable write and the returned result.
func (c *Coordinator) restoreSession(ctx context.Context, snapshot Snapshot, present bool) (View, error) {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return View{}, err
	}
	snapshot = normalized
	sessionID := snapshot.Session.ID()
	admission, err := c.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return View{}, err
	}
	defer admission.Release()
	workspace, err := c.resolveSessionWorkspace(snapshot.Session.Workspace().Path())
	if err != nil {
		return View{}, err
	}
	snapshot.Session, err = snapshot.Session.InstallRestoredWorkspace(workspace)
	if err != nil {
		return View{}, err
	}
	sessionReplacement, err := c.prepareSessionRestore(ctx, snapshot.Session)
	if err != nil {
		return View{}, err
	}
	planReplacement, err := c.prepareRestoredPlanReplacement(ctx, sessionID, snapshot.Plan)
	if err != nil {
		return View{}, err
	}
	committedSession := sessionReplacement.State()
	var view View
	err = c.withGoalMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before restore: %w", err)
			}
			// The restored Session may name a different workspace and always owns a
			// different history. Retire the old scratch tree before commit so the
			// replacement can never resolve to incompatible isolated state.
			if c.sandbox != nil {
				if discardErr := c.sandbox.Discard(sessionID); discardErr != nil {
					return fmt.Errorf("sessions: discard sandbox copy before restore: %w", discardErr)
				}
			}
			return c.writes.ApplyRestore(ctx, restorePlan(snapshot, sessionReplacement, planReplacement))
		},
		func(context.Context) error {
			// Restore replaced the whole history, so process-local read evidence
			// from before the restore is stale.
			c.transientState.ForgetSessionContext(sessionID)
			c.publishAggregateMoved([]string{sessionID}, nil)
			if !present {
				return nil
			}
			var viewErr error
			view, viewErr = c.view(committedSession, ActivityIdle)
			return viewErr
		},
	)
	return view, err
}

func (c *Coordinator) prepareSessionRestore(
	ctx context.Context,
	restored session.Session,
) (Replacement, error) {
	current, err := c.sessions.Get(ctx, restored.ID())
	if errors.Is(err, session.ErrNotFound) {
		return InitialReplacement(restored)
	}
	if err != nil {
		return Replacement{}, err
	}
	next, err := current.ReplaceWithRestore(restored, c.now())
	if err != nil {
		return Replacement{}, err
	}
	return NextReplacement(current, next)
}

// RestorePortableSession rebuilds and restores one transport-neutral archive.
// Boundary codecs decode the archive; aggregate reconstruction and invariant
// enforcement belong here with the restore use case.
func (c *Coordinator) RestorePortableSession(ctx context.Context, portable PortableSnapshot) (View, error) {
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		return View{}, err
	}
	if err := c.models.AdmitSelection(snapshot.Session.Selection()); err != nil {
		return View{}, fmt.Errorf("sessions: restored Session model selection is not admitted: %w", err)
	}
	for _, restoredRun := range snapshot.Runs {
		if err := c.models.AdmitSelection(restoredRun.ModelSelection()); err != nil {
			return View{}, fmt.Errorf(
				"sessions: restored Run %q model selection is not admitted: %w",
				restoredRun.ID(), err,
			)
		}
	}
	return c.restoreSession(ctx, snapshot, true)
}
