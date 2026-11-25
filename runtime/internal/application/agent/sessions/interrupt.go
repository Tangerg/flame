package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

const turnCleanupTimeout = 5 * time.Second

type RunExecutionBinding struct {
	RunID            identity.RunID
	SessionID        identity.SessionID
	ExecutorID       string
	CheckpointRootID string
}

func (r RunExecutionBinding) executorRef() runs.ExecutorRef {
	return runs.ExecutorRef{SessionID: r.SessionID, ExecutorID: r.ExecutorID}
}

func (c *Coordinator) ActiveRun(ctx context.Context, sessionID identity.SessionID) (rundomain.Run, bool, error) {
	runs, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return rundomain.Run{}, false, err
	}
	for _, run := range runs {
		if !run.State().IsTerminal() {
			return run, true, nil
		}
	}
	return rundomain.Run{}, false, nil
}

// The caller holds Session admission from the Pending read through publication.
func (c *Coordinator) CommitParkedTermination(
	ctx context.Context,
	sessionID identity.SessionID,
	pending runs.Pending,
	finishedAt time.Time,
	ending rundomain.Ending,
	receipt runs.CancellationReceipt,
) (rundomain.Run, error) {
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return rundomain.Run{}, err
	}
	messages, err := runs.TerminalConversation(ctx, c.runs, sessionID, pending.RootRunID, snapshot.Messages, ending)
	if err != nil {
		return rundomain.Run{}, err
	}
	plan, err := (parkedRunTerminalization{
		sessionID: sessionID, finishedAt: finishedAt,
		ending: ending, pending: pending, snapshot: snapshot,
		conversationMessages: messages,
	}).build()
	if err != nil {
		return rundomain.Run{}, err
	}
	rootRun, _ := plan.RootRun()
	ref := runs.ExecutorRef{SessionID: sessionID, ExecutorID: pending.ExecutorID}
	_, err = runs.StageCancellation(ctx, receipt, ref, func(ctx context.Context) (runs.CancelResult, error) {
		return runs.CancelResult{Run: rootRun}, c.writes.ApplyTerminal(ctx, plan, runs.CancellationKey(receipt))
	})
	if err != nil {
		return rundomain.Run{}, err
	}
	terminalRuns := plan.Runs()
	runIDs := make([]identity.RunID, len(terminalRuns))
	for index, replacement := range terminalRuns {
		runIDs[index] = replacement.State().ID()
	}
	notices := []invalidation.Notice{
		invalidation.InSession(invalidation.Runs, sessionID, runIDs...),
		invalidation.InSession(invalidation.Interrupts, sessionID, pending.RootRunID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	}
	if !rootRun.GoalIncarnationID().IsZero() {
		notices = append(notices, invalidation.InSession(invalidation.Goals, sessionID))
	}
	c.invalidations.Notify(notices...)
	return rootRun, nil
}

func (c *Coordinator) parkedExecutions(ctx context.Context, sessionID identity.SessionID, runIDs []identity.RunID) ([]RunExecutionBinding, error) {
	var out []RunExecutionBinding
	for _, runID := range runIDs {
		pending, found, err := c.interrupts.Get(ctx, runID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		root, ok := pending.RootContinuation()
		if !ok {
			return nil, fmt.Errorf("sessions: parked run %q has no root continuation", pending.RootRunID)
		}
		out = append(out, RunExecutionBinding{
			RunID:            pending.RootRunID,
			SessionID:        sessionID,
			ExecutorID:       pending.ExecutorID,
			CheckpointRootID: root.MemberID,
		})
	}
	return out, nil
}

func (c *Coordinator) releaseExecution(ctx context.Context, r RunExecutionBinding) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), turnCleanupTimeout)
	defer cancel()
	if err := c.executionReleaser.Release(cleanupCtx, r.executorRef()); err != nil {
		return fmt.Errorf("sessions: release executor %q for Run %q: %w", r.ExecutorID, r.RunID, err)
	}
	return nil
}
