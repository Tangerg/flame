package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// DeleteSession retires everything the Session owns while its mutation
// admission is held, removes all durable state atomically, then tears down
// parked executions and process-local markers. The open interrupts are read up
// front so abandoned executions can be canceled after the durable state is gone.
//
// Checkpoint history and the scratch tree are retired BEFORE the aggregate is
// deleted: the Session is their only name, so retiring them afterwards would
// leave state nothing in the catalog could reach whenever the filesystem
// refuses. Inside the commit, a returned error means exactly one thing — the
// Session still exists.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	deletion, err := NewDeletePlan(sessionID)
	if err != nil {
		return err
	}
	sessionID = deletion.SessionID()
	admission, err := c.ClaimSessionMutation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer admission.Release()

	var pending []runs.Pending
	return c.goals.WithSessionMutation(
		ctx,
		[]string{sessionID},
		func(commitCtx context.Context) error {
			open, err := c.listOpenInterrupts(commitCtx, sessionID)
			if err != nil {
				return err
			}
			pending = append(pending, open...)
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before delete: %w", err)
			}
			if err := c.retireSessionResources(sessionID); err != nil {
				return err
			}
			return c.writes.ApplyDelete(commitCtx, deletion)
		},
		func(ctx context.Context) error {
			// The durable cascade is gone as of here, so the signal cannot outrun it.
			// What remains is process-local: an executor that refuses to release
			// leaks nothing a restart would keep.
			c.publishAggregateMoved([]string{sessionID}, nil)
			var cleanupErrs []error
			for _, item := range pending {
				if err := c.releaseExecution(ctx, RunExecutionBinding{
					RunID:      item.RootRunID,
					SessionID:  sessionID,
					ExecutorID: item.ExecutorID,
				}); err != nil {
					cleanupErrs = append(cleanupErrs, err)
				}
			}
			c.transientState.ForgetSession(sessionID)
			return errors.Join(cleanupErrs...)
		},
	)
}

// Checkpoints and isolated copies belong to the current history. Retire them
// before deleting the Session or replacing its history: restored Run identities
// cannot prove that the old file boundaries belong to the replacement. A refusal
// stops the mutation while the original history still names the remaining state.
func (c *Coordinator) retireSessionResources(sessionID string) error {
	if c.checkpoints != nil {
		if err := c.checkpoints.DropSession(sessionID); err != nil {
			return fmt.Errorf("sessions: drop checkpoints for session %q: %w", sessionID, err)
		}
	}
	if c.sandbox != nil {
		if err := c.sandbox.Discard(sessionID); err != nil {
			return fmt.Errorf("sessions: discard sandbox copy for session %q: %w", sessionID, err)
		}
	}
	return nil
}

// restoreSession applies a canonical archive and, when requested, derives its
// session view before releasing the mutation admission. The view projects the
// value this command committed rather than a fresh read, so another mutation
// cannot interleave between the durable write and the returned result.
func (c *Coordinator) restoreSession(ctx context.Context, snapshot Snapshot, present bool) (View, error) {
	if err := snapshot.ValidateToolResults(); err != nil {
		return View{}, err
	}
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
	restore, err := NewRestorePlan(snapshot, sessionReplacement, planReplacement)
	if err != nil {
		return View{}, err
	}
	committedSession := sessionReplacement.State()
	err = c.goals.WithSessionMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before restore: %w", err)
			}
			if err := c.retireSessionResources(sessionID); err != nil {
				return err
			}
			return c.writes.ApplyRestore(ctx, restore)
		},
		func(context.Context) error {
			// Restore replaced the whole history, so process-local read evidence
			// from before the restore is stale.
			c.transientState.ForgetSessionContext(sessionID)
			c.publishAggregateMoved([]string{sessionID}, nil)
			return nil
		},
	)
	if err != nil || !present {
		return View{}, err
	}
	// The view is the command's result, not settlement: it projects the value
	// this command committed while the mutation admission is still held, so no
	// other writer can move the Session between the write and the answer.
	return c.view(committedSession, ActivityIdle)
}

func (c *Coordinator) prepareSessionRestore(
	ctx context.Context,
	restored session.Session,
) (session.Replacement, error) {
	current, err := c.Get(ctx, restored.ID())
	if errors.Is(err, session.ErrNotFound) {
		return session.InitialReplacement(restored)
	}
	if err != nil {
		return session.Replacement{}, err
	}
	next, err := current.ReplaceWithRestore(restored, c.now())
	if err != nil {
		return session.Replacement{}, err
	}
	return session.NextReplacement(current, next)
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
