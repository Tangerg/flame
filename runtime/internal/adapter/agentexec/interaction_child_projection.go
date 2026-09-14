package agentexec

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
	corechat "github.com/Tangerg/scope/core/chat"
)

func (i *interactionSession) sendExecutorRequest(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	ctx, cancel := i.lifetime.bind(ctx)
	defer cancel()
	select {
	case i.lifetime.events <- event:
		return nil
	case <-i.lifetime.releasing:
		return errInteractionReleased
	case <-ctx.Done():
		return ctx.Err()
	}
}

// reconcileCompletedDelegateChildren projects terminal children in postorder.
// Event listeners only wake this check; public Process state is authoritative.
func (i *interactionSession) reconcileCompletedDelegateChildren(
	ctx context.Context,
) (bool, error) {
	i.childProjection.lock()
	defer i.childProjection.unlock()
	i.state.mu.Lock()
	calls := make([]*managedDelegateCall, 0, len(i.state.delegateChildren))
	for _, managed := range i.state.delegateChildren {
		calls = append(calls, managed)
	}
	i.state.mu.Unlock()
	slices.SortFunc(calls, func(left, right *managedDelegateCall) int {
		if depth := int(right.parentRelation.Depth()+1) - int(left.parentRelation.Depth()+1); depth != 0 {
			return depth
		}
		if parent := strings.Compare(left.identity.parentID.String(), right.identity.parentID.String()); parent != 0 {
			return parent
		}
		if left.modelCallSequence != right.modelCallSequence {
			return cmp.Compare(left.modelCallSequence, right.modelCallSequence)
		}
		if left.toolCallIndex != right.toolCallIndex {
			return cmp.Compare(left.toolCallIndex, right.toolCallIndex)
		}
		return strings.Compare(left.childProcessID.String(), right.childProcessID.String())
	})
	// One inspection decides which delegated children have finished. Asking each
	// child separately would compose the answer out of readings taken at
	// different moments, and this reconciliation publishes terminal facts.
	inspection, readable := i.inspectTree(ctx)
	if !readable {
		return false, nil
	}
	progressed := false
	for _, managed := range calls {
		managed.mu.Lock()
		processID := managed.childProcessID
		done := managed.parentToolFinished

		managed.mu.Unlock()
		if done || !processID.Valid() {
			continue
		}
		member, inspected := inspection.Process(processID)
		if !inspected || !member.Snapshot.Status().Terminal() {
			continue
		}
		process, found := i.engine.Process(processID)
		if !found {
			continue
		}
		result, err := process.Await(ctx)
		if err != nil {
			return progressed, fmt.Errorf("agentexec: await delegated child %s: %w", processID, err)
		}
		projected, err := i.projectDelegateTerminal(ctx, managed, result)
		if err != nil {
			return progressed, err
		}
		progressed = progressed || projected

	}
	return progressed, nil
}

func (i *interactionSession) projectDelegateTerminal(
	ctx context.Context,
	managed *managedDelegateCall,
	result agent.Result,
) (bool, error) {
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if result.ProcessID() != managed.childProcessID {
		return false, errors.New("agentexec: delegated result changed child identity")
	}
	if managed.segmentProjected {
		return false, nil
	}
	member := runs.ExecutorMember{
		MemberID: result.ProcessID().String(), ParentID: managed.identity.parentID.String(),
		SpawnCallID: managed.call.ID,
	}
	if result.Status() == agent.StatusCompleted {
		_, present := result.Output()
		if !present {
			return false, errors.New("agentexec: completed delegated child has no output")
		}
		if !managed.assistantProjected {
			committedReply, replyFound := i.committedReplies.lookup(result.ProcessID())
			if !replyFound {
				return false, errors.New("agentexec: completed delegated child has no committed model reply")
			}
			if !messageRequestsTools(committedReply) {
				completion, err := runs.NewAssistantMessageCompleted(committedReply)
				if err != nil {
					return false, fmt.Errorf("agentexec: construct delegated child answer: %w", err)
				}
				if err := i.commitFact(
					ctx, member, completion,
				); err != nil {
					return false, fmt.Errorf("agentexec: commit delegated child answer: %w", err)
				}
			}
			managed.assistantProjected = true
		}
	}
	end, err := i.segmentEnd(result)
	if err != nil {
		return false, err
	}
	if err := i.sendExecutorRequest(ctx, runs.ExecutorEvent{
		Member: member, Payload: end,
	}); err != nil {
		return false, fmt.Errorf("agentexec: publish delegated child terminal: %w", err)
	}
	managed.segmentProjected = true
	i.modelFailures.forget(result.ProcessID())
	i.committedReplies.forget(result.ProcessID())
	return true, nil
}

func messageRequestsTools(message corechat.Message) bool {
	for _, part := range message.Parts {
		if part.Kind == corechat.PartToolCall {
			return true
		}
	}
	return false
}
