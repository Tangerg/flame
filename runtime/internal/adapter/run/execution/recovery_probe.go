package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

func unresumable(
	ctx context.Context,
	continuation runs.ExecutionContinuation,
	loss run.LossReason,
	reason string,
	cause error,
) (runs.WaitingResumption, error) {
	slog.WarnContext(ctx, "execution: waiting execution is not resumable",
		telemetry.SessionID(continuation.SessionID),
		slog.Any("executor.id", continuation.ExecutorID),
		telemetry.RunLoss(string(loss)),
		slog.Any("reason", reason),
		telemetry.Error(cause),
	)
	return runs.UnresumableWaiting(loss), nil
}

func (i *InteractionExecutor) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.ExecutionContinuation,
) (resumption runs.WaitingResumption, err error) {
	finishAssembly, err := i.sessions.beginAssembly()
	if err != nil {
		return runs.WaitingResumption{}, err
	}
	defer finishAssembly()
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return unresumable(ctx, continuation, run.LossWaitingStateUnavailable, "continuation is malformed", err)
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return unresumable(ctx, continuation, run.LossOtherBuild, "checkpoint belongs to another build", nil)
	}
	if continuation.Isolated {
		return unresumable(ctx, continuation, run.LossIsolatedWorkspace, "isolated workspace does not survive executor loss", nil)
	}
	if err := validateRestoreWorkspace(continuation); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return unresumable(ctx, continuation, run.LossWorkspaceUnavailable, "restore workspace is unavailable", err)
		}
		return runs.WaitingResumption{}, err
	}
	state, err := i.loadCheckpointTree(ctx, continuation)
	if err == nil {
		err = requireWaitingBoundary(state)
	}
	if errors.Is(err, runs.ErrExecutorStateLost) {
		return unresumable(ctx, continuation, run.LossWaitingStateUnavailable, "waiting checkpoint cannot be restored", err)
	}
	if err != nil {
		return runs.WaitingResumption{}, err
	}
	eligibility, err := i.probeCapturedContributions(ctx, continuation, state)
	if err != nil || !eligibility.Resumable() {
		return eligibility, err
	}
	return i.probeAssembledTree(ctx, continuation, state)
}

func (i *InteractionExecutor) probeAssembledTree(ctx context.Context, continuation runs.ExecutionContinuation, state interactionCheckpointState) (resumption runs.WaitingResumption, err error) {
	start := state.restoredStart(continuation)
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: continuation.ExecutorID}
	assembled, err := i.assembleInteraction(ctx, ref, start, &state)
	if err != nil {
		return runs.WaitingResumption{}, fmt.Errorf("execution: assemble Interaction checkpoint probe: %w", err)
	}
	defer func() {
		if cleanupErr := i.discardInteraction(assembled); cleanupErr != nil {
			resumption = runs.WaitingResumption{}
			err = errors.Join(err, cleanupErr)
		}
	}()
	if pendingCheckpointSources(assembled, state) {
		return runs.DeferredWaiting(), nil
	}
	if err := assembled.validateContinuationTree(ctx, continuation, state, false); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return unresumable(ctx, continuation, run.LossConfigurationChanged, "waiting tree validation failed", err)
		}
		return runs.WaitingResumption{}, err
	}

	return runs.ResumableWaiting(), nil
}

func (i *InteractionExecutor) probeCapturedContributions(ctx context.Context, continuation runs.ExecutionContinuation, state interactionCheckpointState) (runs.WaitingResumption, error) {
	bindings := slices.Clone(state.modelObservers)
	bindings = append(bindings, state.modelContexts...)
	bindings = append(bindings, state.modelInputs...)
	bindings = append(bindings, state.toolProposals...)
	if state.modelProvider != nil {
		bindings = append(bindings, state.modelProvider.Binding)
	}
	if i.config.CallComposition.Pending(bindings) {
		return runs.DeferredWaiting(), nil
	}
	bindings, err := plugins.BindActivations(bindings, state.bindings)
	if err != nil {
		return runs.WaitingResumption{}, fmt.Errorf("execution: restore captured contribution bindings: %w", err)
	}
	if _, err := i.config.CallComposition.Activations(ctx, bindings); err != nil {
		if errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, plugin.ErrStale) {
			return unresumable(ctx, continuation, run.LossConfigurationChanged, "captured contribution is unavailable", err)
		}
		return runs.WaitingResumption{}, err
	}
	if definite, err := i.config.CallComposition.RestorationFailure(bindings); err != nil {
		if definite {
			return unresumable(ctx, continuation, run.LossConfigurationChanged, "captured contribution restoration failed", err)
		}
		return runs.WaitingResumption{}, err
	}
	return runs.ResumableWaiting(), nil
}

var _ runs.WaitingExecutionResumability = (*InteractionExecutor)(nil)
