package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
)

// A refused probe recovers the whole waiting tree as lost for loss, which the
// lost Run carries. The finer reason and cause stay in the trace: they
// distinguish failure modes for an operator and can carry payload details.
func unresumable(
	ctx context.Context,
	continuation runs.WaitingContinuation,
	loss runs.Loss,
	reason string,
	cause error,
) (runs.WaitingResumption, error) {
	slog.WarnContext(ctx, "execution: waiting execution is not resumable",
		"session.id", continuation.SessionID,
		"executor.id", continuation.ExecutorID,
		"loss", string(loss),
		"reason", reason,
		"error", cause,
	)
	return runs.UnresumableWaiting(loss), nil
}

// CanResumeWaitingExecution probes one exact durable waiting tree without
// publishing or registering a live executor. Invalid, incompatible, or
// unknown-effect state returns false; assembly/probe I/O failures remain errors
// so startup never mutates facts after an inconclusive read. It deliberately
// validates deployment state and product bindings without acquiring a writer.
// It takes no installation admission: it publishes nothing, and its pending
// checkpoint already holds its installation dependencies.
func (i *InteractionExecutor) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (resumption runs.WaitingResumption, err error) {
	finishAssembly, err := i.sessions.beginAssembly()
	if err != nil {
		return runs.WaitingResumption{}, err
	}
	defer finishAssembly()
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return unresumable(ctx, continuation, runs.LossWaitingStateUnavailable, "continuation is malformed", err)
	}
	checkpoint := continuation.Checkpoint
	if !i.acceptsBuild(checkpoint.BuildID) {
		return unresumable(ctx, continuation, runs.LossOtherBuild, "checkpoint belongs to another build", nil)
	}
	if continuation.Isolated {
		return unresumable(ctx, continuation, runs.LossIsolatedWorkspace, "isolated workspace does not survive executor loss", nil)
	}
	if err := validateRestoreWorkspace(continuation); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return unresumable(ctx, continuation, runs.LossWorkspaceUnavailable, "restore workspace is unavailable", err)
		}
		return runs.WaitingResumption{}, err
	}
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return unresumable(ctx, continuation, runs.LossWaitingStateUnavailable, "checkpoint payload cannot be decoded", err)
	}
	rootID, err := agent.ParseProcessID(checkpoint.RootMemberID)
	if err != nil || state.tree.RootID() != rootID {
		return unresumable(ctx, continuation, runs.LossWaitingStateUnavailable, "checkpoint root member does not own its tree", err)
	}
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, rootID.String())
	if err != nil {
		return runs.WaitingResumption{}, err
	}
	if !found {
		return unresumable(ctx, continuation, runs.LossWaitingStateUnavailable, "execution tree head is missing", nil)
	}
	state.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return runs.WaitingResumption{}, err
	}
	snapshots := state.tree.ProcessSnapshots()
	if len(snapshots) == 0 || snapshots[0].ProcessID() != rootID ||
		!isInteractionWaitingBoundary(snapshots[0].Status()) {
		return unresumable(ctx, continuation, runs.LossWaitingStateUnavailable, "checkpoint tree is not at a waiting boundary", nil)
	}
	start := state.restoredStart(continuation)
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: continuation.ExecutorID}
	assembled, err := i.assembleInteraction(ctx, ref, start)
	if err != nil {
		return runs.WaitingResumption{}, fmt.Errorf("execution: assemble Interaction checkpoint probe: %w", err)
	}
	defer func() {
		if cleanupErr := i.discardInteraction(assembled); cleanupErr != nil {
			resumption = runs.WaitingResumption{}
			err = errors.Join(err, cleanupErr)
		}
	}()
	if err := assembled.validateWaitingTree(ctx, continuation, state); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return unresumable(ctx, continuation, runs.LossConfigurationChanged, "waiting tree validation failed", err)
		}
		return runs.WaitingResumption{}, err
	}

	return runs.ResumableWaiting(), nil
}

var _ runs.WaitingExecutionResumability = (*InteractionExecutor)(nil)
