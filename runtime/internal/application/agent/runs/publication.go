package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type treePublisher struct {
	publications *runPublications
	rootSpec     segmentSpec
	owner        *runTreeOwner
}

func (t treePublisher) publishIncremental(
	ctx context.Context,
	route *executorRoute,
	batch reductionBatch,
) error {
	if err := t.validateIncrementalPublication(route, batch); err != nil {
		return err
	}
	for _, reduced := range batch.events {
		if err := t.commitIncrementalReduction(ctx, route, reduced); err != nil {
			return err
		}
		if err := t.append(route, reduced); err != nil {
			return err
		}
	}
	return nil
}

func (t treePublisher) validateIncrementalPublication(
	route *executorRoute,
	batch reductionBatch,
) error {
	if route == nil {
		return errors.New("runs: publish reductions without an executor route")
	}
	if err := validateReductionBatch(batch); err != nil {
		return err
	}
	if err := validateRouteReductionBatch(route, t.rootSpec.SessionID, batch); err != nil {
		return err
	}
	if batch.parkCommit != nil {
		return fmt.Errorf(
			"runs: run %q produced a per-Run park outside the tree barrier",
			route.runID,
		)
	}
	for index, reduced := range batch.events {
		if reduced.Event.Kind().Terminal() {
			return fmt.Errorf(
				"runs: terminal reduction[%d] requires atomic publication",
				index,
			)
		}
	}
	return nil
}

func (t treePublisher) commitIncrementalReduction(
	ctx context.Context,
	route *executorRoute,
	reduced reduction,
) error {
	if reduced.Commit == nil {
		return nil
	}
	// Every durable projection lands before its event is delivered or retained for
	// replay.
	commit := *reduced.Commit
	if commit.CommitID.IsZero() {
		commit.CommitID = newRunCommitID()
	}
	t.owner.bindChildCancellationReceipt(&commit, nil)
	write := func() error { return t.publications.events.CommitEvent(ctx, commit) }
	if err := commitRunBoundary(ctx, commit.SessionID, commit.RunID, write, commit.durability()); err != nil {
		return fmt.Errorf("runs: commit %T: %w", reduced.Event, err)
	}
	for _, write := range commit.Items {
		t.owner.recordChildCancellationItem(route.runID, write.Item())
	}
	return nil
}

func (t treePublisher) publishAuthoritativeAtomically(
	ctx context.Context,
	route *executorRoute,
	batch reductionBatch,
) error {
	if route == nil {
		return errors.New("runs: publish authoritative fact without an executor route")
	}
	if err := validateReductionBatch(batch); err != nil {
		return err
	}
	if err := validateRouteReductionBatch(route, t.rootSpec.SessionID, batch); err != nil {
		return err
	}
	if batch.parkCommit != nil {
		return errors.New("runs: authoritative fact unexpectedly produced a park boundary")
	}
	combined, err := combineAuthoritativeCommit(route, t.rootSpec.SessionID, batch)
	if err != nil {
		return err
	}
	t.owner.bindChildCancellationReceipt(&combined, nil)
	if !combined.isEmpty() {
		if err := combined.Validate(); err != nil {
			return fmt.Errorf("runs: validate authoritative fact: %w", err)
		}
		write := func() error { return t.publications.events.CommitEvent(ctx, combined) }
		if err := commitRunBoundary(ctx, combined.SessionID, combined.RunID, write, combined.durability()); err != nil {
			return fmt.Errorf("runs: commit authoritative fact: %w", err)
		}
		for _, write := range combined.Items {
			t.owner.recordChildCancellationItem(route.runID, write.Item())
		}
	}
	for _, reduced := range batch.events {
		if err := t.append(route, reduced); err != nil {
			return err
		}
	}
	return nil
}

func (t treePublisher) publishTerminalAtomically(
	ctx context.Context,
	route *executorRoute,
	next *reducer,
	batch reductionBatch,
	ending boundaryDurability,
	cancellation CancellationID,
) error {
	if route == nil {
		return errors.New("runs: publish atomic terminal without an executor route")
	}
	if err := validateReductionBatch(batch); err != nil {
		return err
	}
	if err := validateRouteReductionBatch(route, t.rootSpec.SessionID, batch); err != nil {
		return err
	}
	combined, err := combineTerminalEventCommit(batch)
	if err != nil {
		return err
	}
	if route.member.ParentID == "" {
		combined.ObsoleteCheckpointRootID = route.member.MemberID
	}
	if combined.Run.Canceled() {
		combined.cancellation = cancellation
	}
	t.bindTerminalCancellation(&combined)
	t.owner.bindChildCancellationReceipt(&combined, nil)
	if err := combined.Validate(); err != nil {
		return fmt.Errorf("runs: validate atomic terminal: %w", err)
	}
	durability := combined.durability()
	if ending == retryUntilCommitted {
		durability = retryUntilCommitted
	}
	write := func() error { return t.publications.events.CommitEvent(ctx, combined) }
	if err := commitRunBoundary(ctx, combined.SessionID, combined.RunID, write, durability); err != nil {
		return fmt.Errorf("runs: commit atomic terminal: %w", err)
	}
	route.reducer = next
	route.boundary = boundaryTerminal
	// The database owns one indivisible write-set, while the live tree still
	// applies its facts in causal reduction order. Register Item closures before
	// the terminal Run so process-local ownership observes the same ordering.
	for _, write := range combined.Items {
		t.owner.recordChildCancellationItem(route.runID, write.Item())
	}
	t.owner.recordTerminalRun(*combined.Run, combined.cancellation)
	for _, reduced := range batch.events {
		if err := t.append(route, reduced); err != nil {
			return err
		}
	}
	t.publications.publishRunMoved(t.rootSpec.SessionID, route.runID)
	if combined.ChargesGoal() {
		t.publications.publishGoalMoved(t.rootSpec.SessionID)
	}
	return nil
}

func (t treePublisher) bindTerminalCancellation(commit *EventCommit) {
	if commit.cancellation.IsZero() {
		return
	}
	if commit.Run.Lineage().IsChild() {
		commit.winningCancellationKey = t.owner.matchingChildCancellationKey(commit.cancellation, commit.RunID)
		return
	}
	cancellation := t.owner.matchingRootCancellation(commit.cancellation)
	if cancellation == nil || cancellation.receipt == nil {
		return
	}
	commit.winningCancellationKey = cancellation.receipt.Key()
	commit.rootCancellation = &rootCancellationCommit{executor: t.rootSpec.executorRef(), receipt: cancellation.receipt}
}

func combineTerminalEventCommit(batch reductionBatch) (EventCommit, error) {
	combined := EventCommit{}
	envelopeSet := false
	terminalCommits := 0
	for index, reduced := range batch.events {
		commit := reduced.Commit
		if commit == nil {
			continue
		}
		if !envelopeSet {
			combined.RunID = commit.RunID
			combined.SessionID = commit.SessionID
			combined.SegmentID = commit.SegmentID
			envelopeSet = true
		} else if commit.RunID != combined.RunID || commit.SessionID != combined.SessionID ||
			commit.SegmentID != combined.SegmentID {
			return EventCommit{}, fmt.Errorf(
				"runs: atomic terminal event[%d] changes commit ownership",
				index,
			)
		}
		combined.Items = append(combined.Items, commit.Items...)
		combined.ConversationMessages = appendClonedMessages(
			combined.ConversationMessages,
			commit.ConversationMessages...,
		)
		combined.ModelInvocations = append(combined.ModelInvocations, commit.ModelInvocations...)
		combined.ToolInvocations = append(combined.ToolInvocations, commit.ToolInvocations...)
		if commit.Progress != nil {
			if combined.Progress != nil {
				return EventCommit{}, errors.New("runs: atomic terminal batch repeats Run progress")
			}
			progress := *commit.Progress
			combined.Progress = &progress
		}
		if commit.State == StateTerminalize {
			terminalCommits++
			combined.CommitID = commit.CommitID
			combined.State = commit.State
			combined.Run = commit.Run
		}
	}
	if terminalCommits != 1 || combined.Run == nil {
		return EventCommit{}, fmt.Errorf(
			"runs: atomic terminal batch has %d terminal commits",
			terminalCommits,
		)
	}
	return combined, nil
}

func (t treePublisher) append(route *executorRoute, reduced reduction) error {
	event := t.publications.event(route.runID, route.segmentID, reduced)
	if route.runID == t.rootSpec.RunID && reduced.Event.Kind().Terminal() {
		// Root segment.finished is the client's completion boundary, so publication must
		// not outrun terminal maintenance and admission release or an immediate next
		// command sees busy.
		t.owner.hub.deferCloseEvent(event)
	} else {
		if err := t.owner.hub.append(event); err != nil {
			return fmt.Errorf("runs: publish replay position: %w", err)
		}
	}
	if reduced.Nudge != nil {
		t.publications.workspace.Nudge(reduced.Nudge.WorkspaceCWD, reduced.Nudge.Paths)
	}
	return nil
}

func combineAuthoritativeCommit(route *executorRoute, sessionID identity.SessionID, batch reductionBatch) (EventCommit, error) {
	combined := EventCommit{
		RunID: route.runID, SessionID: sessionID, SegmentID: route.segmentID,
		CommitID: newRunCommitID(),
	}
	for index, reduced := range batch.events {
		if reduced.Event.Kind().Terminal() {
			return EventCommit{}, fmt.Errorf(
				"runs: authoritative fact unexpectedly produced terminal event[%d]",
				index,
			)
		}
		if reduced.Commit == nil {
			continue
		}
		if reduced.Commit.State != StateUnchanged || reduced.Commit.Run != nil {
			return EventCommit{}, fmt.Errorf(
				"runs: authoritative fact event[%d] carries a lifecycle transition",
				index,
			)
		}
		if reduced.Commit.Compaction != nil {
			if combined.Compaction != nil {
				return EventCommit{}, errors.New("runs: repeated conversation compaction")
			}
			combined.Compaction = reduced.Commit.Compaction
		}
		if reduced.Commit.ResultPublication != nil {
			if combined.ResultPublication != nil {
				return EventCommit{}, errors.New("runs: repeated result publication")
			}
			combined.ResultPublication = new(*reduced.Commit.ResultPublication)
			combined.ToolResults = cloneToolResults(reduced.Commit.ToolResults)
		}
		if reduced.Commit.ExecutionContext != nil {
			if combined.ExecutionContext != nil {
				return EventCommit{}, errors.New("runs: repeated execution context")
			}
			combined.ExecutionContext = new(reduced.Commit.ExecutionContext.Clone())
		}
		combined.Items = append(combined.Items, reduced.Commit.Items...)
		combined.ConversationMessages = appendClonedMessages(
			combined.ConversationMessages,
			reduced.Commit.ConversationMessages...,
		)
		combined.ModelInvocations = append(
			combined.ModelInvocations,
			reduced.Commit.ModelInvocations...,
		)
		combined.ToolInvocations = append(
			combined.ToolInvocations,
			reduced.Commit.ToolInvocations...,
		)
		if reduced.Commit.Progress != nil {
			if combined.Progress != nil {
				return EventCommit{}, errors.New("runs: authoritative fact repeats Run progress")
			}
			progress := *reduced.Commit.Progress
			combined.Progress = &progress
		}
	}
	return combined, combined.Validate()
}
