package agentexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// InteractionToolResolver builds the exact Tool manifest for one staged root.
// Runtime binds the resolved Session/workspace scope to ctx before calling it;
// resolution must not execute a Tool or call a model. [toolset.Resolver]
// satisfies this port directly.
type InteractionToolResolver interface {
	Manifest(ctx context.Context, group tool.Group) (toolset.Manifest, error)
}

// InteractionToolInterpreter owns product policy facts implied by concrete
// Tool identities. Toolset satisfies this port without importing Agent Framework. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolInterpreter interface {
	SafetyClass(name string) tool.SafetyClass
	UsesStandardPolicy(name string) bool
	ApprovalSubject(name string, arguments tool.Arguments) (string, error)
	ShellCommand(name, arguments string) string
	ProjectOutcome(ctx context.Context, sessionID, name string, succeeded bool) (runs.ExecutionFact, error)
}

// InteractionToolPresenter owns client-facing activity and result projection.
// Its implementation remains in Toolset; Agent Framework sees only ordinary Tools. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolPresenter interface {
	Activity(name string, arguments tool.Arguments) string
	Present(name string, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}

// ToolAuthorizationRequest is the complete pre-call policy input. The
// authorizer may allow, rewrite, deny, or require durable human approval.
type ToolAuthorizationRequest struct {
	SessionID       string
	CWD             string
	CallID          string
	ToolName        string
	Arguments       tool.Arguments
	SafetyClass     tool.SafetyClass
	ApprovalSubject string
	FileMutation    tool.FileMutationScope
	ShellCommand    string
	AutoApproved    bool
	RequireApproval bool
}

// ToolAuthorizationDecision is one definite pre-call decision: the call is
// denied for a stated reason, it waits for a durable human answer, or it
// proceeds — optionally with arguments policy replaced atomically before the
// ToolCallStarted fact is committed. Naming the outcome is the only way to
// build one, so no decision can deny a call and rewrite it too.
type ToolAuthorizationDecision struct {
	reason    string
	arguments *tool.Arguments
	approval  *runs.ApprovalPrompt
}

// AllowTool proceeds with the model's own arguments.
func AllowTool() ToolAuthorizationDecision { return ToolAuthorizationDecision{} }

// AllowToolWithArguments proceeds with arguments policy replaced.
func AllowToolWithArguments(arguments tool.Arguments) ToolAuthorizationDecision {
	return ToolAuthorizationDecision{arguments: &arguments}
}

// DenyTool refuses the call. The reason reaches the model as a recoverable Tool
// result, so a blank one becomes the generic wording rather than nothing.
func DenyTool(reason string) ToolAuthorizationDecision {
	return ToolAuthorizationDecision{reason: toolDenialReason(reason)}
}

// AskToolApproval waits for a durable human answer to prompt.
func AskToolApproval(prompt runs.ApprovalPrompt) (ToolAuthorizationDecision, error) {
	if err := (runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt}).Validate(); err != nil {
		return ToolAuthorizationDecision{}, fmt.Errorf("agentexec: invalid Tool approval plan: %w", err)
	}
	return ToolAuthorizationDecision{approval: &prompt}, nil
}

// Denied reports the refusal and the reason shown to the model.
func (d ToolAuthorizationDecision) Denied() (string, bool) { return d.reason, d.reason != "" }

// EffectiveArguments reports the replacement arguments, when policy supplied any.
func (d ToolAuthorizationDecision) EffectiveArguments() (tool.Arguments, bool) {
	if d.arguments == nil {
		return tool.Arguments{}, false
	}
	return *d.arguments, true
}

// Approval reports the prompt this call waits on, when it waits on one.
func (d ToolAuthorizationDecision) Approval() (runs.ApprovalPrompt, bool) {
	if d.approval == nil {
		return runs.ApprovalPrompt{}, false
	}
	return *d.approval, true
}

func toolDenialReason(reason string) string {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return trimmed
	}
	return "denied by Tool policy"
}

// InteractionToolAuthorizer evaluates Runtime Tool policy and resolves its
// optional durable human response. It plans but never owns the wait lifecycle:
// interactioninput remains the sole Agent Framework ACL. Implementations must be safe
// for concurrent calls from one Tool batch.
type InteractionToolAuthorizer interface {
	AuthorizeTool(ctx context.Context, request ToolAuthorizationRequest) (ToolAuthorizationDecision, error)
	ResolveToolApproval(
		ctx context.Context,
		request ToolAuthorizationRequest,
		prompt runs.ApprovalPrompt,
		resolution interrupt.Resolution,
	) (ToolAuthorizationDecision, error)
}

// InteractionToolHookInput identifies one ordinary Tool lifecycle callback.
// Result and CallError are present only after execution.
type InteractionToolHookInput struct {
	SessionID string
	CWD       string
	CallID    string
	ToolName  string
	Arguments tool.Arguments
	Result    string
	CallError error
}

// InteractionToolHookDecision is the pre-call hook result: the call is denied
// for a stated reason, or it proceeds — escalated to human review, carrying
// rewritten arguments, or both. A denial carries neither, so the outcome names
// itself the same way the authorization decision does.
type InteractionToolHookDecision struct {
	reason          string
	arguments       *tool.Arguments
	requireApproval bool
}

// AllowToolHook proceeds, optionally escalating to human review and optionally
// with arguments a hook rewrote.
func AllowToolHook(requireApproval bool, arguments *tool.Arguments) InteractionToolHookDecision {
	return InteractionToolHookDecision{requireApproval: requireApproval, arguments: arguments}
}

// DenyToolHook refuses the call, naming the hook when the hook did not.
func DenyToolHook(reason string) InteractionToolHookDecision {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return InteractionToolHookDecision{reason: trimmed}
	}
	return InteractionToolHookDecision{reason: "denied by a PreToolUse hook"}
}

// Denied reports the refusal and the reason shown to the model.
func (d InteractionToolHookDecision) Denied() (string, bool) { return d.reason, d.reason != "" }

// EffectiveArguments reports the rewritten arguments, when a hook supplied any.
func (d InteractionToolHookDecision) EffectiveArguments() (tool.Arguments, bool) {
	if d.arguments == nil {
		return tool.Arguments{}, false
	}
	return *d.arguments, true
}

// RequiresApproval reports a hook escalating a call the gate would have passed.
func (d InteractionToolHookDecision) RequiresApproval() bool { return d.requireApproval }

// InteractionToolHooks owns Runtime lifecycle extensions around ordinary Tool
// calls. PostToolUse is observational: its error is recorded by the caller but
// cannot rewrite a Tool result after the external operation completed. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolHooks interface {
	BeforeToolUse(ctx context.Context, input InteractionToolHookInput) (InteractionToolHookDecision, error)
	AfterToolUse(ctx context.Context, input InteractionToolHookInput) error
}
