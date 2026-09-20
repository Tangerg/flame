package agentexec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
)

// unresumable refuses one probe and says why. A refusal recovers the whole
// waiting tree as lost, which the user sees as a parked Run disappearing, and
// the eleven conditions that produce it are not equally expected: a checkpoint
// from another build is routine after an upgrade, while state this build wrote
// and cannot read is a defect. That distinction exists nowhere but here.
func unresumable(
	ctx context.Context,
	continuation runs.WaitingContinuation,
	reason string,
	cause error,
) (bool, error) {
	slog.WarnContext(ctx, "agentexec: waiting execution is not resumable",
		"session.id", continuation.Checkpoint.Scope.SessionID,
		"executor.id", continuation.ExecutorID,
		"reason", reason,
		"error", cause,
	)
	return false, nil
}

// CanResumeWaitingExecution probes one exact durable waiting tree without
// publishing or registering a live executor. Invalid, incompatible, or
// unknown-effect state returns false; assembly/probe I/O failures remain errors
// so startup never mutates facts after an inconclusive read. It deliberately
// validates deployment state and product bindings without acquiring a writer.
func (i *InteractionExecutor) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (resumable bool, err error) {
	finishAssembly, err := i.sessions.beginAssembly()
	if err != nil {
		return false, err
	}
	defer finishAssembly()
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return unresumable(ctx, continuation, "continuation is malformed", err)
	}
	checkpoint := continuation.Checkpoint
	if !i.acceptsBuild(checkpoint.BuildID) {
		return unresumable(ctx, continuation, "checkpoint belongs to another build", nil)
	}
	if checkpoint.Scope.Isolated {
		return unresumable(ctx, continuation, "isolated workspace does not survive executor loss", nil)
	}
	if err := i.validateRestoreScope(checkpoint.Scope); err != nil {
		return unresumable(ctx, continuation, "restore workspace is unavailable", err)
	}
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return unresumable(ctx, continuation, "checkpoint payload cannot be decoded", err)
	}
	rootID, err := agent.ParseProcessID(checkpoint.RootMemberID)
	if err != nil || state.tree.RootID() != rootID {
		return unresumable(ctx, continuation, "checkpoint root member does not own its tree", err)
	}
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, rootID.String())
	if err != nil {
		return false, err
	}
	if !found {
		return unresumable(ctx, continuation, "execution tree head is missing", nil)
	}
	state.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return false, err
	}
	snapshots := state.tree.ProcessSnapshots()
	if len(snapshots) == 0 || snapshots[0].ProcessID() != rootID ||
		!isInteractionWaitingBoundary(snapshots[0].Status()) {
		return unresumable(ctx, continuation, "checkpoint tree is not at a waiting boundary", nil)
	}
	start := runs.RootExecutionStart{
		SessionID:                checkpoint.Scope.SessionID,
		CWD:                      checkpoint.Scope.CWD,
		WorkspaceCWD:             checkpoint.Scope.WorkspaceCWD,
		Isolated:                 checkpoint.Scope.Isolated,
		GoalIncarnationID:        checkpoint.Scope.GoalIncarnationID,
		ModelSelection:           checkpoint.ModelSelection,
		InterruptKinds:           continuation.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: continuation.ChildRunAdmissionEnabled,
		WorkingContext:           cloneChatMessages(state.instructions),
	}
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: continuation.ExecutorID}
	assembled, err := i.assembleInteraction(ctx, ref, start)
	if err != nil {
		return false, fmt.Errorf("agentexec: assemble Interaction checkpoint probe: %w", err)
	}
	defer func() {
		if cleanupErr := i.discardInteraction(assembled); cleanupErr != nil {
			resumable = false
			err = errors.Join(err, cleanupErr)
		}
	}()
	if err := assembled.validateWaitingTree(ctx, continuation, state); err != nil {
		return unresumable(ctx, continuation, "waiting tree validation failed", err)
	}

	return true, nil
}

var _ runs.WaitingExecutionResumability = (*InteractionExecutor)(nil)
