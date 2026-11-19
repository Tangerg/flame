package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type DroppedRun struct {
	Run       run.Run
	UserInput []transcript.ContentBlock
}

type RollbackResult struct {
	Session View
	Dropped []DroppedRun
}

// RollbackCompletion spans the history change and its receipt while the
// Session, Goal drive and generated-title claims are still held.
type RollbackCompletion func(context.Context, func(context.Context) (RollbackResult, error)) error

func (c *Coordinator) RollbackHistory(ctx context.Context, sessionID, toRunID string, complete RollbackCompletion) error {
	admission, err := c.ClaimSessionMutation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer admission.Release()
	if _, err := c.Get(ctx, sessionID); err != nil {
		return err
	}
	boundary, err := c.resolveRollbackBoundary(ctx, sessionID, toRunID)
	if err != nil {
		return err
	}
	return c.completeRollback(ctx, sessionID, boundary, true, complete)
}

type resolvedRollbackBoundary struct {
	timeline transcript.Boundary
	runs     []run.Run
}

func (c *Coordinator) resolveRollbackBoundary(ctx context.Context, sessionID, toRunID string) (resolvedRollbackBoundary, error) {
	runs, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	boundary, err := transcript.TimelineFromRuns(runs).BoundaryAt(toRunID)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	return resolvedRollbackBoundary{timeline: boundary, runs: runs}, nil
}

// A kept continuation would resume into a tree different from the one its
// executor owns. Only the continuations this rollback removes can be disposed.
func (r resolvedRollbackBoundary) survivingContinuation(restoreHistory bool) (run.Run, bool) {
	dropped := make(map[string]struct{})
	if restoreHistory {
		for _, node := range r.timeline.Dropped {
			dropped[node.ID] = struct{}{}
		}
	}
	for _, value := range r.runs {
		if value.State().IsTerminal() {
			continue
		}
		if _, cut := dropped[value.ID()]; !cut {
			return value, true
		}
	}
	return run.Run{}, false
}

func (c *Coordinator) completeRollback(ctx context.Context, sessionID string, boundary resolvedRollbackBoundary, restoreHistory bool, complete RollbackCompletion) error {
	if !restoreHistory || len(boundary.timeline.Dropped) == 0 {
		return complete(ctx, func(tx context.Context) (RollbackResult, error) {
			return c.rollbackResult(tx, sessionID, nil)
		})
	}
	return c.applyRollback(ctx, sessionID, boundary.timeline, func(ctx context.Context, plan RollbackPlan) error {
		return complete(ctx, func(tx context.Context) (RollbackResult, error) {
			items, err := c.transcript.List(tx, sessionID)
			if err != nil {
				return RollbackResult{}, err
			}
			dropped := boundary.projectDroppedRuns(transcript.OpeningUserMessagesByRun(items))
			if err := c.writes.ApplyRollback(tx, plan); err != nil {
				return RollbackResult{}, err
			}
			return c.rollbackResult(tx, sessionID, dropped)
		})
	})
}

func (c *Coordinator) rollbackResult(ctx context.Context, sessionID string, dropped []DroppedRun) (RollbackResult, error) {
	current, err := c.Get(ctx, sessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	activities, err := c.Activities(ctx, []string{sessionID})
	if err != nil {
		return RollbackResult{}, err
	}
	view, err := c.view(current, activities[sessionID])
	if err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{Session: view, Dropped: dropped}, nil
}

func (r resolvedRollbackBoundary) projectDroppedRuns(inputs map[string][]transcript.ContentBlock) []DroppedRun {
	byID := make(map[string]run.Run, len(r.runs))
	for _, value := range r.runs {
		byID[value.ID()] = value
	}
	dropped := make([]DroppedRun, 0, len(r.timeline.Dropped))
	for _, node := range r.timeline.Dropped {
		dropped = append(dropped, DroppedRun{Run: byID[node.ID], UserInput: inputs[node.ID]})
	}
	return dropped
}

// applyRollback truncates the chat history log to the boundary watermark and drops each run's
// durable record + dangling interrupt as ONE atomic write-set, then cancels
// any in-process parked executions that were abandoned. Delegated work is represented
// by first-class child Runs in this same session, so there is no parallel hidden
// Session tree to infer or purge. An unknown boundary watermark leaves the log
// untouched rather than guessing at a boundary that was never recorded. An
// empty boundary (nothing dropped) is a no-op.
func (c *Coordinator) applyRollback(ctx context.Context, sessionID string, boundary transcript.Boundary, commit func(context.Context, RollbackPlan) error) error {
	dropRunIDs := boundary.DroppedRunIDs()
	// Read the parked executions BEFORE the write-set consumes their interrupts — the
	// in-process executions still need canceling once the durable records are gone.
	parked, err := c.parkedExecutions(ctx, sessionID, dropRunIDs)
	if err != nil {
		return err
	}
	// Read the boundary's Plan BEFORE the write-set drops the runs after it: the
	// kept run survives, but reading inside the write-set would mean persistence deciding
	// which boundary the state comes from.
	plan, err := c.planBoundary(ctx, boundary.KeepRunID)
	if err != nil {
		return err
	}
	planReplacement, err := c.prepareBoundaryPlanReplacement(ctx, sessionID, plan)
	if err != nil {
		return err
	}
	rollback, err := NewRollbackPlan(
		sessionID,
		boundary,
		parkedCheckpointRootIDs(parked),
		planReplacement,
	)
	if err != nil {
		return fmt.Errorf("sessions: prepare rollback: %w", err)
	}
	// A dropped parked run held the session's durable admission slot; dropping its
	// record releases the slot, so the session can start a fresh run afterward.
	return c.goals.WithSessionMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before rollback: %w", err)
			}
			// The scratch tree contains effects from the history being removed.
			// Discard before commit so a successful rollback can never expose the
			// old copy to a later Run. Ordering it after the commit would trade that
			// for the opposite failure: a discard that fails leaves the removed
			// effects in place under a history that no longer records them. This way
			// a failed commit costs the kept Runs' scratch files, which the next Run
			// re-copies from the real workspace, and the rollback reports its error.
			if err := c.sandbox.Discard(sessionID); err != nil {
				return fmt.Errorf("sessions: discard sandbox copy before history rollback: %w", err)
			}
			return c.titles.commitHistoryChange(ctx, sessionID, func(runID string) bool { return slices.Contains(dropRunIDs, runID) }, func() error {
				return commit(ctx, rollback)
			})
		},
		func(ctx context.Context) error {
			// The truncation is committed: the dropped Run subtree is gone and the
			// boundary's Plan is published. Delegated work has no parallel
			// Session identity to clean up.
			c.transientState.ForgetSessionContext(sessionID)
			c.publishAggregateMoved([]string{sessionID}, dropRunIDs)
			var cleanupErrs []error
			for _, r := range parked {
				if err := c.releaseExecution(ctx, r); err != nil {
					cleanupErrs = append(cleanupErrs, err)
				}
			}
			return errors.Join(cleanupErrs...)
		},
	)
}

func parkedCheckpointRootIDs(parked []RunExecutionBinding) []string {
	ids := make([]string, 0, len(parked))
	for _, binding := range parked {
		if binding.CheckpointRootID != "" {
			ids = append(ids, binding.CheckpointRootID)
		}
	}
	return ids
}
