package execution

import (
	"context"
	"errors"
	"fmt"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	scoperun "github.com/Tangerg/scope/agent/run"
)

// PrepareContinuation reads the acknowledged release boundary. It does not assemble
// resources, claim a local boundary, acquire a writer, or deliver a Signal.
func (i *InteractionExecutor) PrepareContinuation(ctx context.Context, continuation runs.ExecutionContinuation, answers []runs.InterruptAnswer, candidates []runs.RememberedRuleCandidate) ([]approval.RememberRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := continuation.Validate(); err != nil {
		return nil, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return nil, fmt.Errorf("%w: continuation belongs to another build", runs.ErrExecutorStateLost)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if _, err := i.session(ref); errors.Is(err, runs.ErrExecutorNotLive) {
		if err := validateRestoreWorkspace(continuation); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	checkpoint, err := decodeRunCheckpoint(continuation.Checkpoint, continuation.SessionID, continuation.RootRunID)
	if err != nil {
		return nil, fmt.Errorf("%w: decode continuation handoff: %w", runs.ErrExecutorStateLost, err)
	}
	root := checkpoint.handoff.rootID
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, root.String())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: execution tree head is missing", runs.ErrExecutorStateLost)
	}
	tree, err := decodeExecutionTree(head, root)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", runs.ErrExecutorStateLost, err)
	}
	retired, err := continuationRetiredProcesses(checkpoint.handoff, tree, continuation.Members)
	if err != nil {
		return nil, err
	}
	if _, err := planContinuationRelease(checkpoint.handoff, tree, retired, answers); err != nil {
		return nil, err
	}
	return i.rememberedApprovalRequests(checkpoint.handoff, tree, retired, candidates)
}

func (i *InteractionExecutor) rememberedApprovalRequests(handoff interactionHandoff, tree scoperun.Log, retired map[scoperun.ProcessID]bool, candidates []runs.RememberedRuleCandidate) ([]approval.RememberRequest, error) {
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, err
	}
	requests := make([]approval.RememberRequest, 0, len(candidates))
	for _, candidate := range candidates {
		matched := false
		for _, target := range handoff.inputs {
			if retired[target.processID] {
				continue
			}
			waiting, approvalWait, err := capturedApproval(inspection, target)
			if err != nil {
				return nil, err
			}
			if !approvalWait || waiting.CallID != candidate.CallID {
				continue
			}
			request, err := i.rememberedApprovalRequest(target, waiting, candidate)
			if err != nil {
				return nil, err
			}
			requests = append(requests, request)
			matched = true
			break
		}
		if !matched {
			return nil, fmt.Errorf("%w: remembered invocation has no current wait", runs.ErrExecutorStateLost)
		}
	}
	return requests, nil
}

func capturedApproval(inspection scoperun.TreeInspection, target interactionInputTarget) (runs.ApprovalPrompt, bool, error) {
	process, found := inspection.Process(target.processID)
	if !found {
		return runs.ApprovalPrompt{}, false, errors.New("execution: input process is absent")
	}
	for _, wait := range process.Waits {
		if wait.Key != target.waitKey {
			continue
		}
		prompt, err := runinput.DecodePrompt(wait.Question.JSON())
		if err != nil {
			return runs.ApprovalPrompt{}, false, err
		}
		approval, ok := prompt.(runs.ApprovalPrompt)
		return approval, ok, nil
	}
	return runs.ApprovalPrompt{}, false, nil
}

func (i *InteractionExecutor) rememberedApprovalRequest(target interactionInputTarget, waiting runs.ApprovalPrompt, candidate runs.RememberedRuleCandidate) (approval.RememberRequest, error) {
	if waiting.Tool != candidate.Request.Tool || waiting.SourceFingerprint != candidate.Request.SourceFingerprint {
		return approval.RememberRequest{}, fmt.Errorf("%w: remembered invocation authority differs from its wait", runs.ErrExecutorStateLost)
	}
	if !target.approvalInputSchema.Valid() {
		return approval.RememberRequest{}, fmt.Errorf("%w: approval input schema is missing", runs.ErrExecutorStateLost)
	}
	if err := target.approvalInputSchema.Validate([]byte(candidate.EffectiveArguments.Canonical())); err != nil {
		return approval.RememberRequest{}, fmt.Errorf("%w: remembered invocation does not satisfy its Tool schema: %w", runs.ErrInvalidInterruptResponse, err)
	}
	subject, err := i.config.ToolInterpreter.ApprovalSubject(candidate.Request.Tool, candidate.EffectiveArguments)
	if err != nil {
		if errors.Is(err, tool.ErrInvalidArguments) {
			return approval.RememberRequest{}, fmt.Errorf("%w: remembered invocation arguments: %w", runs.ErrInvalidInterruptResponse, err)
		}
		return approval.RememberRequest{}, fmt.Errorf("execution: derive remembered Tool approval subject: %w", err)
	}
	request := candidate.Request
	request.Subject = approval.InvocationSubject(subject)
	return request, nil
}

func continuationRetiredProcesses(cut interactionHandoff, head scoperun.Log, members []runs.ExecutionMember) (map[scoperun.ProcessID]bool, error) {
	survivors := make(map[scoperun.ProcessID]runs.ExecutionMember)
	for _, member := range members {
		id, err := scoperun.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, err
		}
		captured := cut.progress[id]
		if captured.Status != scoperun.StatusPaused || captured.OwnerID != member.MemberID {
			return nil, fmt.Errorf("%w: continuation member has no captured paused owner", runs.ErrExecutorStateLost)
		}
		survivors[id] = member
	}
	for id, member := range survivors {
		parent, child := id.Parent()
		if id == cut.rootID {
			if child || member.ParentRunID != "" {
				return nil, fmt.Errorf("%w: root lineage changed", runs.ErrExecutorStateLost)
			}
			continue
		}
		for child && cut.progress[parent].OwnerID != parent.String() {
			parent, child = parent.Parent()
		}
		if !child || survivors[parent].RunID != member.ParentRunID {
			return nil, fmt.Errorf("%w: child lineage changed", runs.ErrExecutorStateLost)
		}
	}
	retiredRoots := make(map[scoperun.ProcessID]bool)
	for id, progress := range cut.progress {
		if progress.OwnerID == id.String() && id != cut.rootID {
			_, survives := survivors[id]
			retiredRoots[id] = !survives
		}
	}
	return retiredHandoffProcesses(cut, head, retiredRoots), nil
}
