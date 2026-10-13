package execution

import (
	"context"
	"errors"
	"fmt"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

// PrepareContinuation reads the acknowledged paused head. It does not assemble
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
	checkpoint, err := decodeExecutorCheckpoint(continuation.Checkpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: decode continuation handoff: %w", runs.ErrExecutorStateLost, err)
	}
	root, err := agent.ParseProcessID(continuation.Checkpoint.RootMemberID)
	if err != nil || root != checkpoint.handoff.rootID {
		return nil, fmt.Errorf("%w: continuation root differs from its handoff", runs.ErrExecutorStateLost)
	}
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
	if err := validateAcceptedPausedTree(checkpoint.handoff, tree, retired); err != nil {
		return nil, err
	}
	members := make(map[agent.ProcessID]runs.ExecutionMember)
	for _, member := range continuation.Members {
		id, _ := agent.ParseProcessID(member.MemberID)
		members[id] = member
	}
	if _, err := restoreInteractionAccounting(checkpoint, members); err != nil {
		return nil, fmt.Errorf("%w: continuation accounting: %w", runs.ErrExecutorStateLost, err)
	}
	if _, err := continuationAnswerPlans(checkpoint.handoff, tree, retired, answers); err != nil {
		return nil, err
	}
	requests := make([]approval.RememberRequest, 0, len(candidates))
	inputs, err := interaction.PendingToolInputs(tree)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		matched := false
		for _, input := range inputs {
			if retired[input.ProcessID()] {
				continue
			}
			prompt, err := runinput.DecodePrompt(input.Prompt())
			if err != nil {
				return nil, err
			}
			if prompt.Approval == nil || prompt.Approval.CallID != candidate.CallID {
				continue
			}
			if prompt.Approval.Tool != candidate.Request.Tool || prompt.Approval.SourceFingerprint != candidate.Request.SourceFingerprint {
				return nil, fmt.Errorf("%w: remembered invocation authority differs from its wait", runs.ErrExecutorStateLost)
			}
			contract := checkpoint.handoff.inputs[input.WaitID()].approvalInputSchema
			if !contract.Valid() {
				return nil, fmt.Errorf("%w: approval input schema is missing", runs.ErrExecutorStateLost)
			}
			if err := contract.Validate([]byte(candidate.EffectiveArguments.Canonical())); err != nil {
				return nil, fmt.Errorf("%w: remembered invocation does not satisfy its Tool schema: %w", runs.ErrInvalidInterruptResponse, err)
			}
			subject, err := i.config.ToolInterpreter.ApprovalSubject(candidate.Request.Tool, candidate.EffectiveArguments)
			if err != nil {
				if errors.Is(err, tool.ErrInvalidArguments) {
					return nil, fmt.Errorf("%w: remembered invocation arguments: %w", runs.ErrInvalidInterruptResponse, err)
				}
				return nil, fmt.Errorf("execution: derive remembered Tool approval subject: %w", err)
			}
			request := candidate.Request
			request.Subject = approval.InvocationSubject(subject)
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

func continuationRetiredProcesses(cut interactionHandoff, head agent.TreeSnapshot, members []runs.ExecutionMember) (map[agent.ProcessID]bool, error) {
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot)
	for _, snapshot := range head.ProcessSnapshots() {
		snapshots[snapshot.Relation().ProcessID()] = snapshot
	}
	survivors := make(map[agent.ProcessID]runs.ExecutionMember)
	for _, member := range members {
		id, err := agent.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, err
		}
		snapshot, found := snapshots[id]
		if !found || snapshot.Status() != agent.StatusPaused {
			return nil, fmt.Errorf("%w: continuation member is not paused", runs.ErrExecutorStateLost)
		}
		survivors[id] = member
	}
	for id, member := range survivors {
		parent, child := snapshots[id].Relation().ParentID()
		if id == cut.rootID {
			if child || member.ParentRunID != "" {
				return nil, fmt.Errorf("%w: continuation root lineage changed", runs.ErrExecutorStateLost)
			}
		} else if !child || survivors[parent].RunID != member.ParentRunID {
			return nil, fmt.Errorf("%w: continuation member lineage changed", runs.ErrExecutorStateLost)
		}
	}
	retiredRoots := make(map[agent.ProcessID]bool)
	for id := range survivors {
		children, _, err := interaction.ActiveDelegateChildren(snapshots[id])
		if err != nil {
			return nil, fmt.Errorf("%w: inspect continuation delegates: %w", runs.ErrExecutorStateLost, err)
		}
		for _, child := range children {
			if _, survives := survivors[child.ProcessID()]; !survives {
				// Product membership owns retirement. Scope may still be draining
				// a canceled child after its product terminal has committed.
				retiredRoots[child.ProcessID()] = true
			}
		}
	}
	return retiredHandoffProcesses(cut, head, retiredRoots), nil
}
