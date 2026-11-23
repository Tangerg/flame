package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type toolAdmission struct {
	binding     toolcontract.Binding
	executable  toolcontract.Tool
	ref         tool.Ref
	fingerprint framevalue.Digest
	interpreter interactionToolInterpreter
	authorizer  interactionToolAuthorizer
	hooks       interactionToolHooks
	input       InteractionToolHookInput
}

func (a toolAdmission) prepare(ctx context.Context, callID string) (ToolAuthorizationRequest, ToolAuthorizationDecision, error) {
	arguments := a.input.Arguments
	hooked, err := a.hooks.BeforeToolUse(ctx, a.input)
	if err != nil {
		return ToolAuthorizationRequest{}, ToolAuthorizationDecision{}, fmt.Errorf("execution: run pre-Tool hook: %w", err)
	}
	if rewritten, changed := hooked.EffectiveArguments(); changed {
		arguments = rewritten
	}
	if reason, denied := hooked.Denied(); denied {
		return ToolAuthorizationRequest{Arguments: arguments}, denyTool(reason), nil
	}
	forceApproval := hooked.RequiresApproval()
	request := ToolAuthorizationRequest{SessionID: a.input.SessionID, WorkspaceCWD: a.input.WorkspaceCWD, CallID: callID, Tool: a.ref, QualifiedName: a.binding.Contract().Definition().Name != a.ref.ModelName(), SourceFingerprint: a.fingerprint, Arguments: arguments,
		SafetyClass: a.interpreter.SafetyClass(a.ref), FileMutation: fileMutationScope(a.executable, arguments, a.input.CWD), ShellCommand: a.interpreter.ShellCommand(a.ref, arguments.Canonical()), RequireApproval: forceApproval}
	if _, err := a.binding.Contract().Prepare(toolcontract.Request{Name: request.Tool.PublishedName(request.QualifiedName), Arguments: arguments.Canonical()}, toolcontract.Admission{}); err != nil {
		return request, ToolAuthorizationDecision{}, errors.Join(tool.ErrInvalidArguments, err)
	}
	if !a.interpreter.UsesStandardPolicy(a.ref) {
		if forceApproval {
			return request, denyTool("a lifecycle hook requires approval, but approval is unavailable"), nil
		}
		return request, allowTool(), nil
	}
	if _, err := a.interpreter.ApprovalSubject(a.ref, arguments); err != nil {
		return request, ToolAuthorizationDecision{}, err
	}
	decision, err := a.authorizer.AuthorizeTool(ctx, request)
	if err != nil {
		return request, ToolAuthorizationDecision{}, fmt.Errorf("execution: authorize Tool %q: %w", request.Tool.PublishedName(request.QualifiedName), err)
	}
	return request, decision, nil
}
