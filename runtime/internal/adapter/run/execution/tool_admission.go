package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Tool admission owns hook rewrites, schema validation and the policy request.
// Run input and human commands retain their separate response lifecycles.
type toolAdmission struct {
	binding     toolcontract.Binding
	executable  toolcontract.Tool
	ref         tool.Ref
	fingerprint string
	interpreter InteractionToolInterpreter
	authorizer  InteractionToolAuthorizer
	hooks       InteractionToolHooks
	input       InteractionToolHookInput
}

func (a toolAdmission) prepare(ctx context.Context, callID string) (ToolAuthorizationRequest, ToolAuthorizationDecision, error) {
	arguments := a.input.Arguments
	forceApproval := false
	if a.hooks != nil {
		decision, err := a.hooks.BeforeToolUse(ctx, a.input)
		if err != nil {
			return ToolAuthorizationRequest{}, ToolAuthorizationDecision{}, fmt.Errorf("execution: run pre-Tool hook: %w", err)
		}
		if rewritten, changed := decision.EffectiveArguments(); changed {
			arguments = rewritten
		}
		if reason, denied := decision.Denied(); denied {
			return ToolAuthorizationRequest{Arguments: arguments}, DenyTool(reason), nil
		}
		forceApproval = decision.RequiresApproval()
	}
	request := ToolAuthorizationRequest{SessionID: a.input.SessionID, WorkspaceCWD: a.input.WorkspaceCWD, CallID: callID, Tool: a.ref, SourceFingerprint: a.fingerprint, ToolName: a.input.ToolName, Arguments: arguments,
		SafetyClass: a.interpreter.SafetyClass(a.ref), FileMutation: fileMutationScope(a.executable, arguments, a.input.CWD), ShellCommand: a.interpreter.ShellCommand(a.ref, arguments.Canonical()), RequireApproval: forceApproval}
	if _, err := a.binding.Contract().Prepare(chat.ToolCall{ID: callID, Name: request.ToolName, Arguments: arguments.Canonical()}); err != nil {
		return request, ToolAuthorizationDecision{}, errors.Join(tool.ErrInvalidArguments, err)
	}
	if !a.interpreter.UsesStandardPolicy(a.ref) {
		if forceApproval {
			return request, DenyTool("a lifecycle hook requires approval, but approval is unavailable"), nil
		}
		return request, AllowTool(), nil
	}
	if _, err := a.interpreter.ApprovalSubject(a.ref, arguments); err != nil {
		return request, ToolAuthorizationDecision{}, err
	}
	decision, err := a.authorizer.AuthorizeTool(ctx, request)
	if err != nil {
		return request, ToolAuthorizationDecision{}, fmt.Errorf("execution: authorize Tool %q: %w", request.ToolName, err)
	}
	return request, decision, nil
}
