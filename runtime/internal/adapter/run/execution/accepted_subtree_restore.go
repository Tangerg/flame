package execution

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	scoperun "github.com/Tangerg/scope/agent/run"
)

type subtreeCancellationRestore struct {
	targetID    scoperun.ProcessID
	reason      string
	disposition runs.WaitingSubtreeDisposition
}

func (s subtreeCancellationRestore) bindMembers(continuation runs.ExecutionContinuation, checkpoint interactionCheckpointState) (runs.ExecutionContinuation, error) {
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return runs.ExecutionContinuation{}, err
	}
	retired := s.retiredProcesses(checkpoint)
	if _, _, err := acceptedReleaseBoundary(checkpoint.handoff, inspection, retired); err != nil {
		if s.disposition == runs.WaitingSubtreeResumesRunning && checkpoint.handoff.rootID == inspection.Root {
			admitted, afterExit, evidenceErr := recordedWaitingSubtreeCancellation(checkpoint.tree, s.targetID, scopeCancellationReason(s.reason))
			if evidenceErr != nil {
				return runs.ExecutionContinuation{}, evidenceErr
			}
			if admitted && survivedScopeResume(checkpoint.handoff, retired, afterExit) {
				return runs.ExecutionContinuation{}, runs.ErrSubtreeCancellationReleased
			}
		}
		return runs.ExecutionContinuation{}, err
	}
	exited := make(map[string]bool, len(retired))
	for _, entry := range checkpoint.tree.Entries {
		if record, ok := entry.Record.(scoperun.Exited); ok && retired[record.Process] {
			exited[record.Process.String()] = true
		}
	}
	continuation.Members = slices.DeleteFunc(slices.Clone(continuation.Members), func(member runs.ExecutionMember) bool {
		return exited[member.MemberID]
	})
	return runs.NewExecutionContinuation(continuation)
}

func survivedScopeResume(cut interactionHandoff, retired map[scoperun.ProcessID]bool, afterExit []scoperun.Entry) bool {
	for _, entry := range afterExit {
		control, ok := entry.Record.(scoperun.ControlRequested)
		if !ok || control.Origin != scoperun.ControlOriginHost || control.Kind != scoperun.ControlResume || retired[control.Process] {
			continue
		}
		if prior, ok := cut.progress[control.Process]; ok && prior.Status == scoperun.StatusPaused {
			return true
		}
	}
	return false
}

func (s subtreeCancellationRestore) retiredProcesses(checkpoint interactionCheckpointState) map[scoperun.ProcessID]bool {
	return retiredHandoffProcesses(checkpoint.handoff, checkpoint.tree, map[scoperun.ProcessID]bool{s.targetID: true})
}

func (s subtreeCancellationRestore) retireProjection(session *interactionSession, checkpoint interactionCheckpointState) {
	retired := s.retiredProcesses(checkpoint)
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	for process, managed := range session.state.delegateChildren {
		if retired[process] {
			managed.mu.Lock()
			managed.segmentProjected = true
			managed.mu.Unlock()
		}
	}
}

type stagedSubtreeCancellation struct {
	session     *interactionSession
	targetID    scoperun.ProcessID
	reason      string
	disposition runs.WaitingSubtreeDisposition
}

func (i *InteractionExecutor) StageAcceptedSubtreeCancellation(
	ctx context.Context,
	request runs.WaitingSubtreeCancellationRequest,
	disposition runs.WaitingSubtreeDisposition,
) (runs.RecoveredSubtreeCancellation, error) {
	if !disposition.Valid() {
		return nil, fmt.Errorf("execution: invalid staged subtree cancellation disposition %q", disposition)
	}
	continuation := request.Continuation()
	if err := continuation.Validate(); err != nil {
		return nil, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return nil, fmt.Errorf("%w: accepted subtree cancellation belongs to another build", runs.ErrExecutorStateLost)
	}
	targetID, err := scoperun.ParseProcessID(request.TargetMemberID())
	if err != nil {
		return nil, err
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	boundary := interactionBoundaryWaiting
	if disposition == runs.WaitingSubtreeResumesRunning {
		boundary = interactionBoundaryAccepted
	}
	restore := subtreeCancellationRestore{targetID: targetID, reason: request.Reason(), disposition: disposition}
	if err := i.restoreContinuationTree(ctx, ref, continuation, boundary, &restore); err != nil {
		return nil, err
	}
	session, err := i.session(ref)
	if err != nil {
		return nil, err
	}
	return stagedSubtreeCancellation{session: session, targetID: targetID, reason: request.Reason(), disposition: disposition}, nil
}

func (s stagedSubtreeCancellation) Settle(ctx context.Context) error {
	if s.disposition == runs.WaitingSubtreeResumesRunning {
		if err := s.session.state.begin(interactionBoundaryAccepted); err != nil {
			return err
		}
		s.session.segmentClock.start()
	}
	if err := s.session.recoverInputHandoff(ctx); err != nil {
		return err
	}
	if err := s.session.settleWaitingSubtree(ctx, s.targetID, s.reason); err != nil {
		return err
	}
	if s.disposition == runs.WaitingSubtreeStaysWaiting {
		s.session.startWorkers()
		return nil
	}
	paused, err := s.session.pausedProcessIDs(ctx)
	if err != nil {
		return err
	}
	s.session.releaseContinuation(nil)
	s.session.startWorkers()
	return s.session.resumePausedProcesses(ctx, paused)
}
