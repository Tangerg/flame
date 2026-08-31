package agentexec

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
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
		return errors.New("agentexec: execution released before executor request")
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
	type delegateBatch struct {
		parentID          agent.ProcessID
		modelCallSequence uint32
	}
	blocked := make(map[delegateBatch]struct{})
	progressed := false
	for _, managed := range calls {
		managed.mu.Lock()
		processID := managed.childProcessID
		done := managed.parentToolFinished
		batch := delegateBatch{
			parentID:          managed.identity.parentID,
			modelCallSequence: managed.modelCallSequence,
		}
		managed.mu.Unlock()
		if done || !processID.Valid() {
			continue
		}
		if _, predecessorPending := blocked[batch]; predecessorPending {
			continue
		}
		process, found := i.engine.Process(processID)
		if !found || !process.Status().Terminal() {
			blocked[batch] = struct{}{}
			continue
		}
		result, err := process.Await(ctx)
		if err != nil {
			return progressed, fmt.Errorf("agentexec: await delegated child %s: %w", processID, err)
		}
		if err := i.projectDelegateResult(ctx, managed, result); err != nil {
			return progressed, err
		}
		progressed = true
	}
	return progressed, nil
}

func (i *interactionSession) projectDelegateResult(
	ctx context.Context,
	managed *managedDelegateCall,
	result agent.Result,
) error {
	committedReply, replyFound := i.committedReplies.lookup(result.ProcessID())
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if result.ProcessID() != managed.childProcessID {
		return errors.New("agentexec: delegated result changed child identity")
	}
	member := runs.ExecutorMember{
		MemberID: result.ProcessID().String(), ParentID: managed.identity.parentID.String(),
		SpawnCallID: managed.call.ID,
	}
	var modelResult corechat.ToolResult
	var childFailure error
	if result.Status() == agent.StatusCompleted {
		erased, present := result.Output()
		if !present {
			return errors.New("agentexec: completed delegated child has no output")
		}
		if !managed.assistantProjected {
			if !replyFound {
				return errors.New("agentexec: completed delegated child has no committed model reply")
			}
			if !messageRequestsTools(committedReply) {
				if err := i.commitFact(
					ctx, member, runs.AssistantMessageCompleted{Message: committedReply},
				); err != nil {
					return fmt.Errorf("agentexec: commit delegated child answer: %w", err)
				}
			}
			managed.assistantProjected = true
		}
		output, err := corechat.NewJSONToolOutput(erased.JSON())
		if err != nil {
			return fmt.Errorf("agentexec: encode delegated child result: %w", err)
		}
		modelResult = corechat.ToolResult{
			ID: managed.call.ID, Name: managed.call.Name, Output: output,
		}
	} else {
		termination := result.Termination()
		diagnostic := "child ended with " + result.Status().String() +
			" (" + termination.Cause().String() + ")"
		if termination.Reason() != "" {
			diagnostic += ": " + termination.Reason()
		}
		childFailure = errors.New("delegated " + diagnostic)
		modelResult = delegateFailureModelResult(managed.call, diagnostic)
	}
	if !managed.segmentProjected {
		if err := i.sendExecutorRequest(ctx, runs.ExecutorEvent{
			Member: member, Payload: i.segmentEnd(result),
		}); err != nil {
			return fmt.Errorf("agentexec: publish delegated child terminal: %w", err)
		}
		managed.segmentProjected = true
		i.committedReplies.forget(result.ProcessID())
	}
	if err := i.finishDelegateTool(ctx, managed, modelResult, childFailure); err != nil {
		return err
	}
	return nil
}

func messageRequestsTools(message corechat.Message) bool {
	for _, part := range message.Parts {
		if part.Kind == corechat.PartToolCall {
			return true
		}
	}
	return false
}

func (i *interactionSession) finishDelegateTool(
	ctx context.Context,
	managed *managedDelegateCall,
	modelResult corechat.ToolResult,
	cause error,
) error {
	if managed.parentToolFinished {
		return nil
	}
	if !managed.toolStarted {
		return errors.New("agentexec: cannot finish a Delegate Tool before its start")
	}
	if err := modelResult.Validate(); err != nil ||
		modelResult.ID != managed.call.ID || modelResult.Name != managed.call.Name {
		return errors.New("agentexec: Delegate Tool result differs from its model call")
	}
	output, textual := modelResult.Output.Text()
	if !textual {
		return errors.New("agentexec: Delegate Tool result is not textual")
	}
	result := tool.StringResult(output)
	if parsed, err := tool.ParseResult([]byte(output)); err == nil {
		result = parsed
	}
	exactModelResult := modelResult.Clone()
	fact := runs.ToolCallFinished{
		CallID: managed.callID.String(), Arguments: managed.arguments.Canonical(),
		ModelResult: &exactModelResult, Result: &result,
	}
	if cause != nil {
		fact.Failure = &tool.Failure{
			Kind:   tool.FailureExecution,
			Detail: executorDiagnostic(cause),
		}
	}
	if err := i.commitFact(ctx, i.executorMember(managed.parentRelation), fact); err != nil {
		return fmt.Errorf("agentexec: commit Delegate Tool result: %w", err)
	}
	managed.parentToolFinished = true
	return nil
}

// delegateFailureModelResult mirrors Interaction's documented Delegate result
// contract at the Runtime projection boundary. Runtime must persist the exact
// model-visible value, not reconstruct it later from the client transcript.
func delegateFailureModelResult(call corechat.ToolCall, diagnostic string) corechat.ToolResult {
	diagnostic = strings.TrimSpace(diagnostic)
	if diagnostic == "" {
		diagnostic = "Interaction operation failed"
	}
	const maximumDiagnosticBytes = 2048
	if len(diagnostic) > maximumDiagnosticBytes {
		diagnostic = diagnostic[:maximumDiagnosticBytes]
		for !utf8.ValidString(diagnostic) {
			diagnostic = diagnostic[:len(diagnostic)-1]
		}
		diagnostic = strings.TrimSpace(diagnostic)
		if diagnostic == "" {
			diagnostic = "Interaction operation failed"
		}
	}
	return corechat.ToolResult{
		ID: call.ID, Name: call.Name,
		Output:  corechat.NewTextToolOutput("error: delegated worker " + diagnostic),
		IsError: true,
	}
}

func delegateStartFailureModelResult(
	call corechat.ToolCall,
	code string,
	message string,
) corechat.ToolResult {
	// Agent Failure bounds its message before Interaction builds the Delegate
	// diagnostic. Retaining that ordering keeps the durable value byte-identical.
	const maximumAgentFailureBytes = 4096
	message = strings.TrimSpace(message)
	if message == "" {
		message = "unknown error"
	}
	if len(message) > maximumAgentFailureBytes {
		message = message[:maximumAgentFailureBytes]
	}
	return delegateFailureModelResult(
		call,
		"child start failed: "+code+": "+message,
	)
}
