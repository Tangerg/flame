package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framecall "github.com/Tangerg/frame/call"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type preparedCall struct {
	arguments tool.Arguments
	failure   error
	input     *toolcontract.InputRequest
	// approved reports that the user approved the call, by answering its prompt
	// or through a remembered rule, rather than the policy passing it.
	approved bool
}

func (p preparedCall) runs() bool { return p.failure == nil && p.input == nil }

func refuseCall(arguments tool.Arguments, reason string) (preparedCall, error) {
	failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
		Kind: toolcontract.FailureKindRejected, Output: toolcontract.NewTextOutput(reason),
	})
	if err != nil {
		return preparedCall{}, hostFailure(err)
	}
	return preparedCall{arguments: arguments}, failure
}

func (o *observedInteractionTool) prepare(
	ctx context.Context,
	bound toolcontract.Invocation,
	callID string,
	name string,
	arguments tool.Arguments,
	continued *runinput.Continuation,
) (preparedCall, error) {
	prepared, err := o.decideCall(ctx, bound, callID, name, arguments, continued)
	if err == nil {
		return prepared, nil
	}
	if _, known := toolcontract.ClassifyFailure(err); !known {
		return preparedCall{}, err
	}
	return preparedCall{arguments: prepared.arguments, failure: err}, nil
}

func (o *observedInteractionTool) decideCall(
	ctx context.Context,
	bound toolcontract.Invocation,
	callID string,
	name string,
	arguments tool.Arguments,
	continued *runinput.Continuation,
) (preparedCall, error) {
	if continued != nil {
		return o.resumePreparedTool(ctx, callID, name, *continued)
	}
	if len(o.session.toolProposals) > 0 {
		proposed, err := o.proposedArguments(ctx, callID, name, arguments)
		if err != nil {
			return preparedCall{}, err
		}
		arguments = proposed
	}
	admission := toolAdmission{
		binding: o.binding, executable: o.inner, ref: o.ref, fingerprint: o.sourceFingerprint,
		interpreter: o.interpreter, authorizer: o.authorizer, hooks: o.hooks,
		input: InteractionToolHookInput{SessionID: o.start.SessionID, CWD: o.start.CWD, WorkspaceCWD: o.start.WorkspaceCWD, ToolName: name, Arguments: arguments},
	}
	if o.scriptCatalog != nil {
		admission.declared = o.declaredApproval
	}
	request, decision, err := admission.prepare(ctx, callID)
	if err != nil {
		if errors.Is(err, tool.ErrInvalidArguments) {
			failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: toolcontract.NewTextOutput("invalid effective arguments: " + run.Diagnostic(err))})
			if failureErr != nil {
				return preparedCall{}, hostFailure(failureErr)
			}
			return preparedCall{arguments: request.Arguments}, failure
		}
		return preparedCall{}, hostFailure(err)
	}
	arguments = request.Arguments
	if rewritten, ok := decision.EffectiveArguments(); ok {
		arguments = rewritten
	}
	if reason, denied := decision.Denied(); denied {
		return refuseCall(arguments, reason)
	}
	if prompt, ok := decision.Approval(); ok {
		return o.requestToolApproval(ctx, bound, request, prompt)
	}
	// Either requirement makes the policy ask, so passing means a remembered rule approved.
	return preparedCall{arguments: arguments, approved: request.RequireApproval || request.ScriptTools != ""}, nil
}

func (o *observedInteractionTool) proposedArguments(ctx context.Context, callID, name string, arguments tool.Arguments) (tool.Arguments, error) {
	activations, err := o.session.callComposition.Activations(ctx, o.session.compositionBindings())
	if err != nil {
		return tool.Arguments{}, err
	}
	if !slices.EqualFunc(activations, o.session.compositionActivations, plugin.Activation.Equal) {
		return tool.Arguments{}, plugin.ErrStale
	}
	invocation, err := operation.Capture(ctx, framecall.Target{Scope: extension.ScopeCall, Reference: callID})
	if err != nil {
		return tool.Arguments{}, err
	}
	calling, err := pluginmcp.WithInvocation(ctx, invocation)
	if err != nil {
		return tool.Arguments{}, err
	}
	proposal, err := o.session.callComposition.PrepareToolProposal(calling, o.session.toolProposals, corechat.ToolCall{ID: callID, Name: name, Arguments: arguments.Canonical()})
	if err != nil {
		return tool.Arguments{}, err
	}
	return tool.ParseArguments(proposal.Arguments)
}

func (o *observedInteractionTool) validateApprovalSubject(name string, arguments tool.Arguments) error {
	if _, err := o.interpreter.ApprovalSubject(o.ref, arguments); err != nil {
		cause := fmt.Errorf("execution: derive Tool %q approval subject: %w", name, err)
		if errors.Is(err, tool.ErrInvalidArguments) {
			failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{
				Kind: toolcontract.FailureKindFailed, Cause: cause,
				Output: toolcontract.NewTextOutput("invalid effective arguments: " + run.Diagnostic(cause)),
			})
			if failureErr != nil {
				return hostFailure(failureErr)
			}
			return failure
		}
		return hostFailure(cause)
	}
	return nil
}

func (o *observedInteractionTool) authorizationRequest(
	callID string,
	name string,
	arguments tool.Arguments,
) (ToolAuthorizationRequest, error) {
	if err := o.validateApprovalSubject(name, arguments); err != nil {
		return ToolAuthorizationRequest{}, err
	}
	return ToolAuthorizationRequest{
		SessionID: o.start.SessionID, WorkspaceCWD: o.start.WorkspaceCWD,
		CallID: callID, Tool: o.ref, QualifiedName: name != o.ref.ModelName(), SourceFingerprint: o.sourceFingerprint, Arguments: arguments,
		SafetyClass:  o.interpreter.SafetyClass(o.ref),
		FileMutation: fileMutationScope(o.inner, arguments, o.start.CWD),
		ShellCommand: o.interpreter.ShellCommand(o.ref, arguments.Canonical()),
	}, nil
}

func (o *observedInteractionTool) requestToolApproval(
	ctx context.Context,
	bound toolcontract.Invocation,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
) (preparedCall, error) {
	if !slices.Contains(o.start.InterruptTypes, run.InterruptApproval) {
		return refuseCall(request.Arguments, "approval input is unavailable for this Run")
	}
	if err := validateToolApprovalPrompt(request, prompt); err != nil {
		return preparedCall{}, fmt.Errorf("execution: invalid Tool approval prompt: %w", err)
	}
	resolution, inputRequest, err := runinput.Require(ctx, bound, prompt)
	if err != nil {
		return preparedCall{}, err
	}
	if inputRequest != nil {
		return preparedCall{input: inputRequest}, nil
	}
	return o.resolveToolApproval(ctx, request, prompt, resolution)
}

func (o *observedInteractionTool) resumePreparedTool(
	ctx context.Context,
	callID string,
	name string,
	continued runinput.Continuation,
) (preparedCall, error) {
	storedName, storedArguments := continued.Interrupt.ToolCall()
	if storedName != name {
		return preparedCall{}, errors.New("execution: continued Tool input belongs to another Tool")
	}
	arguments, err := tool.ParseArguments(storedArguments)
	if err != nil {
		return preparedCall{}, fmt.Errorf("execution: parse continued Tool arguments: %w", err)
	}
	var prompt runs.ApprovalPrompt
	switch interrupt := continued.Interrupt.(type) {
	case runs.QuestionPrompt:
		return preparedCall{arguments: arguments}, nil
	case runs.ApprovalPrompt:
		prompt = interrupt
	default:
		return preparedCall{}, errors.New("execution: continued Tool input has an unsupported kind")
	}
	if prompt.CallID != callID {
		return preparedCall{}, errors.New("execution: continued Tool approval call identity changed")
	}
	request, err := o.authorizationRequest(callID, name, arguments)
	if err != nil {
		return preparedCall{arguments: arguments}, err
	}
	return o.resolveToolApproval(ctx, request, prompt, continued.Resolution)
}

func (o *observedInteractionTool) resolveToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
	resolution run.InterruptResolution,
) (preparedCall, error) {
	decision, err := o.authorizer.ResolveToolApproval(ctx, request, prompt, resolution)
	if err != nil {
		return preparedCall{}, hostFailure(fmt.Errorf("execution: resolve Tool %q approval: %w", request.Tool.PublishedName(request.QualifiedName), err))
	}
	arguments := request.Arguments
	if rewritten, ok := decision.EffectiveArguments(); ok {
		arguments = rewritten
	}
	if reason, denied := decision.Denied(); denied {
		return refuseCall(arguments, reason)
	}
	if _, err := o.prepareInvocation(corechat.ToolCall{
		ID: request.CallID, Name: request.Tool.PublishedName(request.QualifiedName), Arguments: arguments.Canonical(),
	}, arguments, toolcontract.Admission{}); err != nil {
		return preparedCall{arguments: arguments}, err
	}
	// Arguments edited in the answer meet the subject rule that admitted the originals.
	if err := o.validateApprovalSubject(request.Tool.PublishedName(request.QualifiedName), arguments); err != nil {
		return preparedCall{arguments: arguments}, err
	}
	return preparedCall{arguments: arguments, approved: true}, nil
}
