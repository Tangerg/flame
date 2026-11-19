package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// ViewCompletion keeps the mutation and its exact reply in the same durable
// decision before a caller can observe the publication.
type ViewCompletion func(context.Context, func(context.Context) (View, error)) error

// DeleteSession commits history deletion and its resource retirement together.
// Only the committed responsibility authorizes destruction; failed cleanup
// retains a durable admission fence for recovery rather than undoing deletion.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string, complete func(context.Context, func(context.Context) error) error) error {
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
			open, err := c.interrupts.List(commitCtx, sessionID)
			if err != nil {
				return err
			}
			pending = open
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before delete: %w", err)
			}
			return c.titles.commitHistoryChange(commitCtx, sessionID, func(string) bool { return true }, func() error {
				return complete(commitCtx, func(tx context.Context) error {
					return c.writes.ApplyDelete(tx, deletion)
				})
			})
		},
		func(ctx context.Context) error {
			// The durable cascade is gone as of here, so the signal cannot outrun it.
			// Executor release is independent of durable resource retirement;
			// a refusal must not suppress either cleanup responsibility.
			c.publishAggregateMoved([]string{sessionID}, nil)
			cleanupErrs := []error{c.retirer.complete(ctx, deletion.Retirement())}
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

func (c *Coordinator) RestorePortableSession(ctx context.Context, portable PortableSnapshot, complete ViewCompletion) error {
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		return err
	}
	sessionID := snapshot.Session.ID()
	admission, err := c.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return err
	}
	defer admission.Release()
	workspace, err := c.resolveSessionWorkspace(snapshot.Session.Workspace().Path())
	if err != nil {
		return err
	}
	snapshot.Session, err = snapshot.Session.InstallRestoredWorkspace(workspace)
	if err != nil {
		return err
	}
	planReplacement, err := c.prepareRestoredPlanReplacement(ctx, sessionID, plan.Document{Steps: snapshot.Plan, Source: snapshot.PlanSource})
	if err != nil {
		return err
	}
	var restore RestorePlan
	return c.goals.WithSessionMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			return c.titles.commitHistoryChange(ctx, sessionID, func(string) bool { return true }, func() error {
				replacement, err := c.prepareSessionRestore(ctx, snapshot.Session)
				if err != nil {
					return err
				}
				restore, err = NewRestorePlan(snapshot, replacement, planReplacement)
				if err != nil {
					return err
				}
				view, err := c.view(replacement.State(), activityIdle)
				if err != nil {
					return err
				}
				if err := c.transientState.QuiesceSession(sessionID); err != nil {
					return fmt.Errorf("sessions: quiesce process-local Session state before restore: %w", err)
				}
				return complete(ctx, func(tx context.Context) (View, error) {
					return view, c.writes.ApplyRestore(tx, restore)
				})
			})
		},
		func(ctx context.Context) error {
			// Restore replaced the whole history, so process-local read evidence
			// from before the restore is stale.
			c.transientState.ForgetSessionContext(sessionID)
			c.publishAggregateMoved([]string{sessionID}, nil)
			if retirement, replaced := restore.Retirement(); replaced {
				return c.retirer.complete(ctx, retirement)
			}
			return nil
		},
	)
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
