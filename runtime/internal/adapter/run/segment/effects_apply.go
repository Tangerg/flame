package segment

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (e *Effects) applyCommit(ctx context.Context, commit runs.EventCommit) error {
	if err := e.runState.RequireActiveSegment(ctx, commit.SessionID, commit.RunID, commit.SegmentID); err != nil {
		return fmt.Errorf("segment: require active event Segment: %w", err)
	}
	if applied, err := e.resultPublicationApplied(ctx, commit); err != nil || applied {
		return err
	}
	commit, err := e.closeTerminalConversation(ctx, commit)
	if err != nil {
		return err
	}
	// One transaction writes these in order; later steps read what earlier
	// ones wrote.
	for _, apply := range []func(context.Context, runs.EventCommit) error{
		e.recordResultPublication,
		e.applyCompaction,
		e.applyItems,
		e.applyConversation,
		e.applyModelInvocations,
		e.applyToolInvocations,
		e.applyProgress,
		e.applyState,
		recordChildCancellation,
		e.recordRunCommit,
	} {
		if err := apply(ctx, commit); err != nil {
			return err
		}
	}
	return nil
}

func (e *Effects) resultPublicationApplied(ctx context.Context, commit runs.EventCommit) (bool, error) {
	publication := commit.ResultPublication
	if publication == nil {
		return false, nil
	}
	return e.runState.ResultPublicationCommitted(ctx, commit.SessionID, commit.RunID, commit.SegmentID, publication.ID, publication.Digest)
}

func (e *Effects) closeTerminalConversation(ctx context.Context, commit runs.EventCommit) (runs.EventCommit, error) {
	if commit.State != runs.StateTerminalize || !commit.Run.Lineage().IsRoot() {
		return commit, nil
	}
	messages, err := e.conversation.Read(ctx, commit.SessionID)
	if err != nil {
		return commit, fmt.Errorf("segment: read terminal conversation: %w", err)
	}
	ending, finished := commit.Run.Ending()
	if !finished {
		return commit, fmt.Errorf("segment: terminal Run %q has no ending", commit.RunID)
	}
	closure, err := runs.TerminalConversation(ctx, e.runState, commit.SessionID, commit.RunID, append(messages, commit.ConversationMessages...), ending)
	if err != nil {
		return commit, err
	}
	commit.ConversationMessages = append(commit.ConversationMessages, closure...)
	return commit, nil
}

func (e *Effects) recordResultPublication(ctx context.Context, commit runs.EventCommit) error {
	publication := commit.ResultPublication
	if publication == nil {
		return nil
	}
	return e.runState.RecordResultPublication(ctx, commit.SessionID, commit.RunID, commit.SegmentID, publication.ID, publication.Digest, commit.ToolResults)
}

func (e *Effects) applyCompaction(ctx context.Context, commit runs.EventCommit) error {
	if commit.Compaction == nil {
		return nil
	}
	return e.compactions.ApplyCompaction(ctx, *commit.Compaction)
}

func (e *Effects) applyItems(ctx context.Context, commit runs.EventCommit) error {
	for _, write := range commit.Items {
		if err := e.writeItem(ctx, write); err != nil {
			return err
		}
	}
	return nil
}

func (e *Effects) applyConversation(ctx context.Context, commit runs.EventCommit) error {
	if len(commit.ConversationMessages) == 0 {
		return nil
	}
	if err := e.conversation.Write(ctx, commit.SessionID, commit.ConversationMessages...); err != nil {
		return fmt.Errorf("segment: append conversation messages: %w", err)
	}
	return nil
}

func recordChildCancellation(ctx context.Context, commit runs.EventCommit) error {
	if err := commit.RecordChildCancellation(ctx); err != nil {
		return fmt.Errorf("segment: record confirmed active child cancellation: %w", err)
	}
	return nil
}

func (e *Effects) recordRunCommit(ctx context.Context, commit runs.EventCommit) error {
	if commit.State != runs.StateUnchanged || commit.CommitID.IsZero() {
		return nil
	}
	if err := e.runState.RecordRunCommit(ctx, commit.SessionID, commit.RunID, commit.SegmentID, commit.CommitID); err != nil {
		return fmt.Errorf("segment: record event commit receipt: %w", err)
	}
	return nil
}

func (e *Effects) applyModelInvocations(ctx context.Context, commit runs.EventCommit) error {
	for _, invocation := range commit.ModelInvocations {
		if err := e.modelInvocations.RecordModelInvocation(ctx, commit.SessionID, commit.RunID, invocation); err != nil {
			return fmt.Errorf("segment: record model invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (e *Effects) applyToolInvocations(ctx context.Context, commit runs.EventCommit) error {
	for _, invocation := range commit.ToolInvocations {
		if err := e.toolInvocations.RecordToolInvocation(ctx, commit.SessionID, commit.RunID, invocation); err != nil {
			return fmt.Errorf("segment: record Tool invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (e *Effects) applyProgress(ctx context.Context, commit runs.EventCommit) error {
	if commit.Progress == nil {
		return nil
	}
	if err := e.runProgress.UpdateProgress(ctx, *commit.Progress); err != nil {
		return fmt.Errorf("segment: update Run progress: %w", err)
	}
	return nil
}

func (e *Effects) writeItem(ctx context.Context, write runs.ItemWrite) error {
	if settlement, ok := write.Settlement(); ok {
		return e.transcript.SettleItem(ctx, settlement)
	}
	return e.transcript.AppendItem(ctx, write.Item())
}

func (e *Effects) applyState(ctx context.Context, commit runs.EventCommit) error {
	if commit.State == runs.StateUnchanged {
		return nil
	}
	switch commit.State {
	case runs.StateSuspend:
		return e.runState.Suspend(ctx, *commit.Run, commit.SegmentID, commit.CommitID)
	case runs.StateTerminalize:
		run, err := e.finishedRun(ctx, *commit.Run)
		if err != nil {
			return err
		}
		if err := e.runState.TerminalizeEvent(ctx, run, commit.SegmentID, commit.CommitID, commit.WinningCancellationKey()); err != nil {
			return err
		}
		if err := commit.RecordRootCancellation(ctx, run); err != nil {
			return fmt.Errorf("segment: record confirmed root cancellation: %w", err)
		}
		if run.Lineage().IsChild() {
			return nil
		}
		if err := e.acceptedContinuations.Delete(ctx, run.ID()); err != nil {
			return err
		}
		if commit.ObsoleteCheckpointRootID == "" {
			return nil
		}
		if err := e.executorCheckpoints.DeleteCheckpoints(ctx, commit.SessionID, []string{commit.ObsoleteCheckpointRootID}); err != nil {
			return fmt.Errorf("segment: delete terminal executor checkpoint %q: %w", commit.ObsoleteCheckpointRootID, err)
		}
		if err := e.interrupts.Delete(ctx, commit.RunID); err != nil {
			return fmt.Errorf("segment: delete terminal interrupt for root Run %q: %w", commit.RunID, err)
		}
		if err := e.childRunStarts.DeleteSession(ctx, commit.SessionID); err != nil {
			return fmt.Errorf("segment: delete terminal child Run start reservations: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("segment: unknown run state change %q", commit.State)
	}
}

func (e *Effects) finishedRun(ctx context.Context, record run.Run) (run.Run, error) {
	mark, needed, err := e.terminalMessageMark(ctx, record)
	if err != nil || !needed {
		return record, err
	}
	finalized, err := record.WithMessageMark(mark)
	if err != nil {
		return run.Run{}, fmt.Errorf("segment: apply terminal message watermark: %w", err)
	}
	return finalized, nil
}

func (e *Effects) terminalMessageMark(ctx context.Context, record run.Run) (int, bool, error) {
	if _, known := record.MessageMark().Count(); !record.Lineage().IsRoot() || known {
		return 0, false, nil
	}
	mark, err := e.conversation.Count(ctx, record.SessionID())
	if err != nil {
		return 0, false, fmt.Errorf("segment: resolve terminal message watermark: %w", err)
	}
	return mark, true, nil
}
