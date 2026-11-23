package execution

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) publishWaitingBoundary() bool {
	i.state.mu.Lock()
	process := i.state.process
	if process == nil || i.state.phase == interactionFinished || i.state.boundary != interactionBoundaryInactive ||
		i.state.cancellationsInFlight > 0 {
		i.state.mu.Unlock()
		return false
	}
	captured := make(chan struct{})
	i.state.barrierCapture = captured
	i.state.mu.Unlock()
	defer func() {
		i.state.mu.Lock()
		i.state.endBarrierCapture(captured)
		i.state.mu.Unlock()
	}()
	ctx := i.lifetime.reconciling
	snapshot, interruptions, found, err := i.captureHumanInputBarrier(ctx)
	if err != nil {
		i.reportProjectionFailure(err)
		return false
	}
	if !found {
		return false
	}
	if _, err := i.reconcileCompletedDelegateChildren(ctx); err != nil {
		i.reportProjectionFailure(err)
		return false
	}
	checkpoint, err := i.executorCheckpoint(snapshot)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	barrier, err := runs.NewTreeInterrupted(checkpoint, interruptions)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	i.state.mu.Lock()
	// The captured cut is the authority on what was waiting; re-reading a live
	// status here would describe a different moment than the checkpoint does.
	if i.state.phase == interactionFinished || i.state.boundary != interactionBoundaryInactive ||
		i.state.process != process {
		i.state.mu.Unlock()
		return false
	}
	i.state.holdBoundary(interactionBoundaryWaiting)
	i.state.waitingCheckpoint = checkpoint.Clone()
	i.state.endBarrierCapture(captured)
	i.state.mu.Unlock()
	published := i.lifetime.send(runs.ExecutorEvent{
		Member:  i.executorMember(process.ID()),
		Payload: barrier,
	})
	if published {
		if err := i.lifecycleHooks.NotifyWaiting(
			i.lifetime.execution, i.start.SessionID, i.start.WorkspaceCWD,
		); err != nil {
			slog.ErrorContext(i.lifetime.execution, "execution: notify waiting",
				telemetry.SessionID(i.start.SessionID), telemetry.Error(err),
			)
		}
	}
	return published
}

func (i *interactionSession) stageAcceptedContinuation(checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase == interactionFinished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryWaiting || i.state.observer != nil {
		return runs.ErrExecutionClaimed
	}
	if !executorCheckpointsEqual(i.state.waitingCheckpoint, checkpoint) {
		return fmt.Errorf("%w: live Interaction checkpoint differs from the claimed waiting boundary", runs.ErrInvalidExecutorCheckpoint)
	}
	i.state.boundary = interactionBoundaryAccepted
	return nil
}

func (i *interactionSession) beginContinuation() (bool, error) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase == interactionFinished {
		return false, runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryAccepted || i.state.observer == nil {
		return false, errors.New("execution: Interaction continuation was not staged and observed")
	}
	if i.state.phase == interactionPending {
		i.state.phase = interactionBegun
		return true, nil
	}
	if i.state.process == nil {
		return false, runs.ErrExecutorNotLive
	}
	return false, nil
}

func isInteractionWaitingBoundary(status scoperun.Status) bool {
	return status == scoperun.StatusWaiting || status == scoperun.StatusPaused
}

func (i *interactionSession) releaseContinuation(input *runs.CommittedUserInput) {
	i.state.mu.Lock()
	if input != nil {
		i.state.pendingResumeInputs = append(i.state.pendingResumeInputs, *input)
	}
	i.state.continueExecution()
	i.state.mu.Unlock()
}

func (i *interactionState) holdBoundary(boundary interactionBoundary) {
	i.boundary = boundary
	i.boundaryGate = make(chan struct{})
}

func (i *interactionState) endBarrierCapture(captured chan struct{}) {
	if i.barrierCapture == captured {
		i.barrierCapture = nil
		close(captured)
	}
}

func (i *interactionState) releaseBoundary() {
	i.boundary = interactionBoundaryInactive
	close(i.boundaryGate)
	i.boundaryGate = nil
}

func (i *interactionState) continueExecution() {
	i.waitingCheckpoint = runs.ExecutorCheckpoint{}
	i.releaseBoundary()
}

func executorCheckpointsEqual(left, right runs.ExecutorCheckpoint) bool {
	return left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID &&
		left.SessionID == right.SessionID &&
		bytes.Equal(left.Payload, right.Payload)
}
