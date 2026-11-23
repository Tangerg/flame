package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
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

func (c *Coordinator) RollbackHistory(ctx context.Context, sessionID identity.SessionID, toRunID identity.RunID, complete operation.Reply[RollbackResult]) error {
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
	return c.completeRollback(ctx, sessionID, boundary, complete)
}

type resolvedRollbackBoundary struct {
	timeline transcript.Boundary
	runs     []run.Run
}

func (c *Coordinator) resolveRollbackBoundary(ctx context.Context, sessionID identity.SessionID, toRunID identity.RunID) (resolvedRollbackBoundary, error) {
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

func (r resolvedRollbackBoundary) keepingHistory() resolvedRollbackBoundary {
	r.timeline.Dropped = nil
	return r
}

func (r resolvedRollbackBoundary) keepsHistory() bool { return len(r.timeline.Dropped) == 0 }

func (r resolvedRollbackBoundary) survivingContinuation() (run.Run, bool) {
	dropped := make(map[identity.RunID]struct{}, len(r.timeline.Dropped))
	for _, node := range r.timeline.Dropped {
		dropped[node.ID] = struct{}{}
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

func (c *Coordinator) completeRollback(ctx context.Context, sessionID identity.SessionID, boundary resolvedRollbackBoundary, complete operation.Reply[RollbackResult]) error {
	if boundary.keepsHistory() {
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

func (c *Coordinator) rollbackResult(ctx context.Context, sessionID identity.SessionID, dropped []DroppedRun) (RollbackResult, error) {
	current, err := c.Get(ctx, sessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	view, err := c.viewForSession(ctx, current)
	if err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{Session: view, Dropped: dropped}, nil
}

func (r resolvedRollbackBoundary) projectDroppedRuns(inputs map[identity.RunID][]transcript.ContentBlock) []DroppedRun {
	byID := make(map[identity.RunID]run.Run, len(r.runs))
	for _, value := range r.runs {
		byID[value.ID()] = value
	}
	dropped := make([]DroppedRun, 0, len(r.timeline.Dropped))
	for _, node := range r.timeline.Dropped {
		dropped = append(dropped, DroppedRun{Run: byID[node.ID], UserInput: inputs[node.ID]})
	}
	return dropped
}

func (c *Coordinator) applyRollback(ctx context.Context, sessionID identity.SessionID, boundary transcript.Boundary, commit func(context.Context, RollbackPlan) error) error {
	dropRunIDs := boundary.DroppedRunIDs()
	// Read the parked executions BEFORE the write-set consumes their interrupts — the
	// in-process executions still need canceling once the durable records are gone.
	parked, err := c.parkedExecutions(ctx, sessionID, dropRunIDs)
	if err != nil {
		return err
	}
	planReplacement, err := c.boundaryPlanReplacement(ctx, sessionID, boundary.KeepRunID)
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
	return c.goals.WithSessionMutation(
		ctx,
		[]identity.SessionID{sessionID},
		func(ctx context.Context) error {
			if err := c.transientState.QuiesceSession(sessionID); err != nil {
				return fmt.Errorf("sessions: quiesce process-local Session state before rollback: %w", err)
			}
			// Discarded before commit so a successful rollback never exposes the old copy; a
			// failed commit only costs scratch files the next Run re-copies.
			if err := c.sandbox.Discard(sessionID); err != nil {
				return fmt.Errorf("sessions: discard sandbox copy before history rollback: %w", err)
			}
			return c.titles.commitHistoryChange(ctx, sessionID, func(runID identity.RunID) bool { return slices.Contains(dropRunIDs, runID) }, func() error {
				return commit(ctx, rollback)
			})
		},
		func(ctx context.Context) error {
			c.transientState.ForgetSessionContext(sessionID)
			c.publishAggregateMoved([]identity.SessionID{sessionID}, dropRunIDs)
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
