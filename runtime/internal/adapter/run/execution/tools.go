package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
)

// Resolution must not execute a Tool or call a model. A successful resolution
// transfers manifest resource ownership to this execution, even if assembly later
// fails.
type interactionToolResolver interface {
	Manifest(ctx context.Context, group tool.Group, retained *toolset.Retained) (toolset.Manifest, error)
}

type interactionToolInterpreter interface {
	SafetyClass(ref tool.Ref) tool.SafetyClass
	UsesStandardPolicy(ref tool.Ref) bool
	ApprovalSubject(ref tool.Ref, arguments tool.Arguments) (string, error)
	ShellCommand(ref tool.Ref, arguments string) string
	ProjectOutcome(ctx context.Context, sessionID identity.SessionID, ref tool.Ref, succeeded bool) (runs.ExecutionFact, error)
}

type interactionToolPresenter interface {
	Present(ref tool.Ref, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}

type ToolAuthorizationRequest struct {
	SessionID identity.SessionID
	// A rule keyed by an isolated Run's scratch copy would never match the project
	// again.
	WorkspaceCWD      string
	CallID            string
	Tool              tool.Ref
	QualifiedName     bool
	SourceFingerprint framevalue.Digest
	Arguments         tool.Arguments
	SafetyClass       tool.SafetyClass
	FileMutation      tool.FileMutationScope
	ShellCommand      string
	RequireApproval   bool
	// ScriptTools is set for run_script; see approval.ToolCallInput.
	ScriptTools tool.SafetyClass
}

// ToolApprovalScope names a Tool without a call, for a script that declares it.
type ToolApprovalScope struct {
	SessionID         identity.SessionID
	WorkspaceCWD      string
	Tool              tool.Ref
	SourceFingerprint framevalue.Digest
	SafetyClass       tool.SafetyClass
}

type ToolAuthorizationDecision struct {
	toolVerdict
	approval *runs.ApprovalPrompt
}

type toolVerdict struct {
	reason    string
	arguments *tool.Arguments
}

func (v toolVerdict) Denied() (string, bool) { return v.reason, v.reason != "" }

func (v toolVerdict) EffectiveArguments() (tool.Arguments, bool) {
	if v.arguments == nil {
		return tool.Arguments{}, false
	}
	return *v.arguments, true
}

func allowTool() ToolAuthorizationDecision { return ToolAuthorizationDecision{} }

func allowToolWithArguments(arguments tool.Arguments) ToolAuthorizationDecision {
	return ToolAuthorizationDecision{toolVerdict: toolVerdict{arguments: &arguments}}
}

func denyTool(reason string) ToolAuthorizationDecision {
	return ToolAuthorizationDecision{toolVerdict: toolVerdict{reason: refusalReason(reason, policyRefusal)}}
}

func askToolApproval(prompt runs.ApprovalPrompt) (ToolAuthorizationDecision, error) {
	if err := prompt.Validate(); err != nil {
		return ToolAuthorizationDecision{}, fmt.Errorf("execution: invalid Tool approval plan: %w", err)
	}
	return ToolAuthorizationDecision{approval: &prompt}, nil
}

func validateToolApprovalPrompt(request ToolAuthorizationRequest, prompt runs.ApprovalPrompt) error {
	if err := validateToolAuthorizationRequest(request); err != nil {
		return err
	}
	if err := prompt.Validate(); err != nil {
		return err
	}
	if prompt.CallID != request.CallID || prompt.Tool != request.Tool || prompt.QualifiedName != request.QualifiedName || prompt.SourceFingerprint != request.SourceFingerprint ||
		prompt.Arguments != request.Arguments.Canonical() || prompt.SafetyClass != request.SafetyClass {
		return errors.New("execution: Tool approval prompt differs from its invocation")
	}
	return nil
}

func (d ToolAuthorizationDecision) Approval() (runs.ApprovalPrompt, bool) {
	if d.approval == nil {
		return runs.ApprovalPrompt{}, false
	}
	return *d.approval, true
}

const (
	policyRefusal = "denied by Tool policy"
	userRefusal   = "tool call denied by user"
	hookRefusal   = "denied by a PreToolUse hook"
)

func refusalReason(reason, fallback string) string {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return trimmed
	}
	return fallback
}

// Implementations must be safe for concurrent calls from one Tool batch. Errors
// are host failures and cannot supply a Tool result.
type interactionToolAuthorizer interface {
	AuthorizeTool(ctx context.Context, request ToolAuthorizationRequest) (ToolAuthorizationDecision, error)
	ResolveToolApproval(
		ctx context.Context,
		request ToolAuthorizationRequest,
		prompt runs.ApprovalPrompt,
		resolution run.InterruptResolution,
	) (ToolAuthorizationDecision, error)
	AsksForTool(ctx context.Context, scope ToolApprovalScope) (bool, error)
}

type InteractionToolHookInput struct {
	SessionID identity.SessionID
	// An isolated copy was never granted the project's hook trust, so hooks resolve
	// from WorkspaceCWD.
	CWD          string
	WorkspaceCWD string
	ToolName     string
	Arguments    tool.Arguments
	Result       string
	CallError    error
}

type InteractionToolHookDecision struct {
	toolVerdict
	requireApproval bool
}

func allowToolHook(requireApproval bool, arguments *tool.Arguments) InteractionToolHookDecision {
	if arguments != nil {
		arguments = new(*arguments)
	}
	return InteractionToolHookDecision{toolVerdict: toolVerdict{arguments: arguments}, requireApproval: requireApproval}
}

func denyToolHook(reason string) InteractionToolHookDecision {
	return InteractionToolHookDecision{toolVerdict: toolVerdict{reason: refusalReason(reason, hookRefusal)}}
}

func (d InteractionToolHookDecision) RequiresApproval() bool { return d.requireApproval }

// PostToolUse is observational and cannot rewrite a completed result.
// Implementations must be safe for concurrent calls from one Tool batch.
type interactionToolHooks interface {
	BeforeToolUse(ctx context.Context, input InteractionToolHookInput) (InteractionToolHookDecision, error)
	AfterToolUse(ctx context.Context, input InteractionToolHookInput) error
}
