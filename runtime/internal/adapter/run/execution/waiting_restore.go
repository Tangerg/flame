package execution

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

func (i *InteractionExecutor) RestoreWaitingExecution(
	ctx context.Context,
	continuation runs.ExecutionContinuation,
) (runs.ExecutorRef, error) {
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return runs.ExecutorRef{}, fmt.Errorf(
			"%w: checkpoint build %q does not match %q",
			runs.ErrExecutorStateLost,
			continuation.Checkpoint.BuildID,
			i.buildID.String(),
		)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if _, err := i.session(ref); err == nil {
		return runs.ExecutorRef{}, runs.ErrExecutionClaimed
	} else if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	if err := i.restoreContinuationTree(
		ctx,
		ref,
		continuation,
		interactionBoundaryWaiting,
		nil,
	); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func (i *InteractionExecutor) restoreContinuationTree(
	ctx context.Context,
	ref runs.ExecutorRef,
	continuation runs.ExecutionContinuation,
	boundary interactionBoundary,
	subtree *subtreeCancellationRestore,
) (err error) {
	finishAssembly, err := i.sessions.beginAssembly()
	if err != nil {
		return err
	}
	defer finishAssembly()
	if err := validateRestoreWorkspace(continuation); err != nil {
		return err
	}
	checkpoint, err := i.loadCheckpointTree(ctx, continuation)
	if err != nil {
		return err
	}
	if boundary == interactionBoundaryWaiting {
		if err := requireWaitingBoundary(checkpoint); err != nil {
			return err
		}
	}
	if subtree != nil {
		continuation, err = subtree.bindMembers(continuation, checkpoint)
		if err != nil {
			return err
		}
	}
	start := checkpoint.restoredStart(continuation)
	session, err := i.assembleInteraction(ctx, ref, start, &checkpoint)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, i.discardInteraction(session))
		}
	}()
	if pendingCheckpointSources(session, checkpoint) {
		return errors.New("execution: required MCP source is still connecting")
	}
	if err := session.validateContinuationTree(ctx, continuation, checkpoint, subtree == nil && boundary == interactionBoundaryAccepted); err != nil {
		return fmt.Errorf("execution: validate continuation tree: %w", err)
	}
	// Recover commits activation without replaying process admission. Its first
	// Store callback must already see the original product ownership projection.
	if err := session.restoreContinuationProjection(continuation, checkpoint, boundary); err != nil {
		return err
	}
	return i.publishRestored(ctx, session, checkpoint, boundary, subtree)
}

func (i *InteractionExecutor) publishRestored(
	ctx context.Context,
	session *interactionSession,
	checkpoint interactionCheckpointState,
	boundary interactionBoundary,
	subtree *subtreeCancellationRestore,
) error {
	if subtree != nil {
		subtree.retireProjection(session, checkpoint)
		return i.publishInteraction(ctx, session)
	}
	if boundary == interactionBoundaryAccepted {
		// A partially released tree can drive Scope immediately on Recover.
		// Begin must attach the product observer before taking its writer.
		return i.publishInteraction(ctx, session)
	}
	return i.publishWaitingInteraction(ctx, session, checkpoint)
}

func (i *InteractionExecutor) loadCheckpointTree(ctx context.Context, continuation runs.ExecutionContinuation) (interactionCheckpointState, error) {
	checkpoint, err := decodeRunCheckpoint(continuation.Checkpoint, continuation.SessionID, continuation.RootRunID)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("%w: parse Interaction checkpoint: %w", runs.ErrExecutorStateLost, err)
	}
	rootID := checkpoint.handoff.rootID
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, rootID.String())
	if err != nil {
		return interactionCheckpointState{}, err
	}
	if !found {
		return interactionCheckpointState{}, fmt.Errorf("%w: execution tree head is missing", runs.ErrExecutorStateLost)
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("%w: %w", runs.ErrExecutorStateLost, err)
	}
	return checkpoint, nil
}

func requireWaitingBoundary(checkpoint interactionCheckpointState) error {
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return err
	}
	if root, found := inspection.Process(checkpoint.handoff.rootID); !found || !isInteractionWaitingBoundary(root.Status) {
		return fmt.Errorf("%w: Interaction restore requires a product waiting boundary", runs.ErrExecutorStateLost)
	}
	return nil
}

func (i *InteractionExecutor) publishWaitingInteraction(ctx context.Context, session *interactionSession, checkpoint interactionCheckpointState) error {
	if err := session.recoverInputHandoff(ctx); err != nil {
		return err
	}
	interruptions, err := session.pendingInterruptions(checkpoint.tree)
	if err != nil {
		return fmt.Errorf("%w: inspect restored Interaction input: %w", runs.ErrExecutorStateLost, err)
	}
	if len(interruptions) == 0 {
		return fmt.Errorf("%w: restored Interaction tree has no pending input", runs.ErrExecutorStateLost)
	}
	if err := i.publishInteraction(ctx, session); err != nil {
		return err
	}
	session.startWorkers()
	return nil
}

func validateRestoreWorkspace(continuation runs.ExecutionContinuation) error {
	if continuation.Isolated {
		return fmt.Errorf("%w: isolated workspaces are not restorable after executor loss", runs.ErrExecutorStateLost)
	}
	path := continuation.Workspace
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: restore workspace %q: %w", runs.ErrExecutorStateLost, path, err)
		}
		return fmt.Errorf("execution: inspect restore workspace %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: restore workspace %q is unavailable", runs.ErrExecutorStateLost, path)
	}
	return nil
}
