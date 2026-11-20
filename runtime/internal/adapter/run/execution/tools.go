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

// interactionToolResolver builds the exact Tool manifest for one staged root.
// Runtime binds the resolved Session/workspace scope to ctx before calling it;
// resolution must not execute a Tool or call a model. A successful resolution
// transfers manifest resource ownership to this execution, including on later
// assembly failure. A nil retained input admits the current catalog; restoration
// supplies original bindings before discovery and collision resolution.
// [toolset.Resolver] satisfies this port directly.
type interactionToolResolver interface {
	Manifest(ctx context.Context, group tool.Group, retained *toolset.Retained) (toolset.Manifest, error)
}

// interactionToolInterpreter owns product policy facts implied by concrete
// Tool identities. Toolset satisfies this port without importing Agent Framework. An
// implementation must be safe for concurrent calls from one Tool batch.
type interactionToolInterpreter interface {
	SafetyClass(ref tool.Ref) tool.SafetyClass
	UsesStandardPolicy(ref tool.Ref) bool
	ApprovalSubject(ref tool.Ref, arguments tool.Arguments) (string, error)
	ShellCommand(ref tool.Ref, arguments string) string
	ProjectOutcome(ctx context.Context, sessionID identity.SessionID, ref tool.Ref, succeeded bool) (runs.ExecutionFact, error)
}

// interactionToolPresenter owns client-facing result projection.
// Its implementation remains in Toolset; Agent Framework sees only ordinary Tools. An
// implementation must be safe for concurrent calls from one Tool batch.
type interactionToolPresenter interface {
	Present(ref tool.Ref, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}

// ToolAuthorizationRequest is the complete pre-call policy input. The
// authorizer may allow, rewrite, deny, or require durable human approval.
type ToolAuthorizationRequest struct {
	SessionID identity.SessionID
	// WorkspaceCWD is the Session's project directory, which is the scope key a
	// remembered approval is stored under. An isolated Run executes elsewhere;
	// a rule keyed by that scratch copy would never match the project again.
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
}

// ToolAuthorizationDecision is one definite pre-call decision: the call is
// denied for a stated reason, it waits for a durable human answer, or it
// proceeds — optionally with arguments policy replaced atomically before the
// ToolCallStarted fact is committed. Naming the outcome is the only way to
// build one, so no decision can deny a call and rewrite it too.
type ToolAuthorizationDecision struct {
	toolVerdict
	approval *runs.ApprovalPrompt
}

// toolVerdict is what every pre-call decision states: a refusal with the reason
// the model sees, or permission with the arguments that replace the model's own.
type toolVerdict struct {
	reason    string
	arguments *tool.Arguments
}

// Denied reports the refusal and the reason shown to the model.
func (v toolVerdict) Denied() (string, bool) { return v.reason, v.reason != "" }

// EffectiveArguments reports the replacement arguments, when any were supplied.
func (v toolVerdict) EffectiveArguments() (tool.Arguments, bool) {
	if v.arguments == nil {
		return tool.Arguments{}, false
	}
	return *v.arguments, true
}

// allowTool proceeds with the model's own arguments.
func allowTool() ToolAuthorizationDecision { return ToolAuthorizationDecision{} }

// allowToolWithArguments proceeds with arguments policy replaced.
func allowToolWithArguments(arguments tool.Arguments) ToolAuthorizationDecision {
	return ToolAuthorizationDecision{toolVerdict: toolVerdict{arguments: &arguments}}
}

// denyTool refuses the call. The reason reaches the model as a recoverable Tool
// result, so a blank one becomes the generic wording rather than nothing.
func denyTool(reason string) ToolAuthorizationDecision {
	return ToolAuthorizationDecision{toolVerdict: toolVerdict{reason: refusalReason(reason, policyRefusal)}}
}

// askToolApproval waits for a durable human answer to prompt.
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

// Approval reports the prompt this call waits on, when it waits on one.
func (d ToolAuthorizationDecision) Approval() (runs.ApprovalPrompt, bool) {
	if d.approval == nil {
		return runs.ApprovalPrompt{}, false
	}
	return *d.approval, true
}

// Generic wording for a denier that gave no reason: the model always learns
// why its call did not run.
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

// interactionToolAuthorizer evaluates Runtime Tool policy and resolves its
// optional durable human response. It plans but never owns the wait lifecycle:
// Run input remains the sole Agent Framework ACL. Implementations must be safe
// for concurrent calls from one Tool batch.
// Errors report a host policy failure and cannot supply a Tool result or input
// request. Only a successful decision can deny or request approval.
type interactionToolAuthorizer interface {
	AuthorizeTool(ctx context.Context, request ToolAuthorizationRequest) (ToolAuthorizationDecision, error)
	ResolveToolApproval(
		ctx context.Context,
		request ToolAuthorizationRequest,
		prompt runs.ApprovalPrompt,
		resolution run.InterruptResolution,
	) (ToolAuthorizationDecision, error)
}

// InteractionToolHookInput identifies one ordinary Tool lifecycle callback.
// Result and CallError are present only after execution.
type InteractionToolHookInput struct {
	SessionID identity.SessionID
	// CWD is where the call ran. WorkspaceCWD is the Session's project
	// directory, which decides which hooks exist and whether the project has
	// been trusted to run them; an isolated copy was never granted that trust.
	CWD          string
	WorkspaceCWD string
	ToolName     string
	Arguments    tool.Arguments
	Result       string
	CallError    error
}

// InteractionToolHookDecision is the pre-call hook result: the call is denied
// for a stated reason, or it proceeds — escalated to human review, carrying
// rewritten arguments, or both. A denial carries neither, so the outcome names
// itself the same way the authorization decision does.
type InteractionToolHookDecision struct {
	toolVerdict
	requireApproval bool
}

// allowToolHook proceeds, optionally escalating to human review and optionally
// with arguments a hook rewrote.
func allowToolHook(requireApproval bool, arguments *tool.Arguments) InteractionToolHookDecision {
	if arguments != nil {
		arguments = new(*arguments)
	}
	return InteractionToolHookDecision{toolVerdict: toolVerdict{arguments: arguments}, requireApproval: requireApproval}
}

// denyToolHook refuses the call, naming the hook when the hook did not.
func denyToolHook(reason string) InteractionToolHookDecision {
	return InteractionToolHookDecision{toolVerdict: toolVerdict{reason: refusalReason(reason, hookRefusal)}}
}

// RequiresApproval reports a hook escalating a call the gate would have passed.
func (d InteractionToolHookDecision) RequiresApproval() bool { return d.requireApproval }

// interactionToolHooks owns Runtime lifecycle extensions around ordinary Tool
// calls. PostToolUse is observational: its error is recorded by the caller but
// cannot rewrite a Tool result after the external operation completed. An
// implementation must be safe for concurrent calls from one Tool batch.
type interactionToolHooks interface {
	BeforeToolUse(ctx context.Context, input InteractionToolHookInput) (InteractionToolHookDecision, error)
	AfterToolUse(ctx context.Context, input InteractionToolHookInput) error
}
