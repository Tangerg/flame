package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// InteractionApprovalPolicy is the exact product policy view required at the
// Interaction Tool boundary.
type InteractionApprovalPolicy interface {
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	Decide(ctx context.Context, query approval.Query) (approval.Decision, bool, error)
	// ErrSourceAuthorityChanged declines persistence without changing the one-shot answer.
	Remember(ctx context.Context, request approval.RememberRequest) error
}

// ToolAuthorizer evaluates Runtime approval policy independently of Agent Framework.
// It returns a durable product prompt when a person is required; the executor
// ACL alone maps that prompt to an Interaction wait and response Signal.
type ToolAuthorizer struct {
	policy   InteractionApprovalPolicy
	subjects approvalSubjectInterpreter
}

type approvalSubjectInterpreter interface {
	ApprovalSubject(tool.Ref, tool.Arguments) (string, error)
}

// NewToolAuthorizer binds the product approval policy.
func NewToolAuthorizer(policy InteractionApprovalPolicy, subjects approvalSubjectInterpreter) (*ToolAuthorizer, error) {
	if dependency.Missing(policy) {
		return nil, errors.New("execution: Tool approval policy is required")
	}
	if dependency.Missing(subjects) {
		return nil, errors.New("execution: Tool approval subject interpreter is required")
	}
	return &ToolAuthorizer{policy: policy, subjects: subjects}, nil
}

func (t *ToolAuthorizer) AuthorizeTool(
	ctx context.Context,
	request ToolAuthorizationRequest,
) (ToolAuthorizationDecision, error) {
	if err := validateToolAuthorizationRequest(request); err != nil {
		return ToolAuthorizationDecision{}, err
	}
	mode, err := t.policy.Mode(ctx, request.SessionID)
	if err != nil {
		return ToolAuthorizationDecision{}, fmt.Errorf("execution: read Tool approval mode: %w", err)
	}
	plan := (approval.ToolCallInput{
		Mode:            mode,
		RequireApproval: request.RequireApproval,
		SafetyClass:     request.SafetyClass,
		FileMutation:    request.FileMutation,
		ShellCommand:    request.ShellCommand,
	}).Plan()
	if plan.Action == approval.GatePrompt {
		subject, err := t.subjects.ApprovalSubject(request.Tool, request.Arguments)
		if err != nil {
			return ToolAuthorizationDecision{}, fmt.Errorf("execution: derive Tool approval subject: %w", err)
		}
		decision, matched, err := t.policy.Decide(ctx, approval.Query{
			SessionID:  request.SessionID,
			ProjectDir: request.WorkspaceCWD,
			Tool:       request.Tool, SourceFingerprint: request.SourceFingerprint,
			Subject: subject,
		})
		if err != nil {
			return ToolAuthorizationDecision{}, fmt.Errorf("execution: evaluate remembered Tool approval: %w", err)
		}
		plan = plan.ResolvePromptShortcuts(
			approval.StandingDecision{Decision: decision, Matched: matched},
		)
	}
	switch plan.Action {
	case approval.GatePass:
		return AllowTool(), nil
	case approval.GateDeny:
		return DenyTool(approvalDenialMessage(plan.Denial, request.ToolName)), nil
	case approval.GatePrompt:
		prompt := runs.ApprovalPrompt{
			CallID: request.CallID,
			Tool:   request.Tool, SourceFingerprint: request.SourceFingerprint,
			ToolName:     request.ToolName,
			Arguments:    request.Arguments.Canonical(),
			SafetyClass:  plan.SafetyClass,
			Risk:         plan.Risk,
			Reason:       approvalPromptReason(plan.PromptCause),
			Rememberable: true,
		}
		return AskToolApproval(prompt)
	default:
		return ToolAuthorizationDecision{}, errors.New("execution: Tool approval policy returned an unknown action")
	}
}

func (t *ToolAuthorizer) ResolveToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
	resolution interrupt.Resolution,
) (ToolAuthorizationDecision, error) {
	if err := validateToolApprovalPrompt(request, prompt); err != nil {
		return ToolAuthorizationDecision{}, err
	}
	arguments := request.Arguments
	if resolution.Approved && resolution.Arguments != "" {
		var err error
		arguments, err = tool.ParseArguments(resolution.Arguments)
		if err != nil {
			return ToolAuthorizationDecision{}, fmt.Errorf("execution: parse approved Tool arguments: %w", err)
		}
	}
	if prompt.Rememberable && resolution.RememberScope != "" {
		subject, err := t.subjects.ApprovalSubject(request.Tool, arguments)
		if err != nil {
			return ToolAuthorizationDecision{}, fmt.Errorf("execution: derive remembered Tool approval subject: %w", err)
		}
		if err := t.policy.Remember(ctx, approval.RememberRequest{
			Scope:      resolution.RememberScope,
			SessionID:  request.SessionID,
			ProjectDir: request.WorkspaceCWD,
			Tool:       request.Tool, SourceFingerprint: request.SourceFingerprint,
			Subject:  approval.InvocationSubject(subject),
			Decision: approval.DecisionOf(resolution.Approved),
		}); err != nil && !errors.Is(err, approval.ErrSourceAuthorityChanged) {
			return ToolAuthorizationDecision{}, fmt.Errorf("execution: remember Tool approval: %w", err)
		}
	}
	if !resolution.Approved {
		return DenyTool(denialReason(resolution.Reason)), nil
	}
	if arguments.Canonical() == request.Arguments.Canonical() {
		return AllowTool(), nil
	}
	return AllowToolWithArguments(arguments), nil
}

func validateToolAuthorizationRequest(request ToolAuthorizationRequest) error {
	if err := request.Tool.ValidateFingerprint(request.SourceFingerprint); err != nil {
		return err
	}
	if request.Tool.ModelName() != request.ToolName {
		return errors.New("execution: tool name differs from its reference")
	}
	if err := validateToolAuthorizationText("SessionID", request.SessionID); err != nil {
		return err
	}
	if err := validateToolAuthorizationText("workspace CWD", request.WorkspaceCWD); err != nil {
		return err
	}
	if err := validateToolAuthorizationText("CallID", request.CallID); err != nil {
		return err
	}
	if err := validateToolAuthorizationText("ToolName", request.ToolName); err != nil {
		return err
	}
	if !request.SafetyClass.Valid() {
		return fmt.Errorf("execution: Tool authorization has invalid safety class %q", request.SafetyClass)
	}
	if !request.FileMutation.Valid() {
		return fmt.Errorf("execution: Tool authorization has invalid file mutation scope %q", request.FileMutation)
	}
	if request.Arguments.Canonical() == "" {
		return errors.New("execution: Tool authorization arguments are required")
	}
	return nil
}

func validateToolAuthorizationText(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("execution: Tool authorization %s is required without surrounding whitespace", name)
	}
	return nil
}

func fileMutationScope(
	executable toolcontract.Tool,
	arguments tool.Arguments,
	cwd string,
) tool.FileMutationScope {
	reporter, found, err := toolcontract.Capability[toolset.FileMutationReporter](executable)
	if err != nil {
		return tool.FileMutationUnknown
	}
	if !found {
		return tool.FileMutationNone
	}
	paths, err := reporter.MutationPaths([]byte(arguments.Canonical()))
	if err != nil {
		return tool.FileMutationUnknown
	}
	if len(paths) == 0 {
		return tool.FileMutationNone
	}
	if strings.TrimSpace(cwd) == "" {
		return tool.FileMutationUnknown
	}
	root, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return tool.FileMutationUnknown
	}
	for _, path := range paths {
		target, err := pathidentity.Resolve(root, path)
		if err != nil {
			return tool.FileMutationUnknown
		}
		inside, err := pathidentity.Contains(root, target)
		if err != nil {
			return tool.FileMutationUnknown
		}
		if !inside {
			return tool.FileMutationOutsideWorkspace
		}
	}
	return tool.FileMutationWithinWorkspace
}

func approvalDenialMessage(denial approval.DenialCause, toolName string) string {
	switch denial {
	case approval.DenialPlanMode:
		return fmt.Sprintf("plan mode is active (read-only): %s is not permitted. Continue investigating with read-only tools or request Plan approval before making changes.", toolName)
	case approval.DenialRememberedRule:
		return "tool call denied by a remembered rule"
	default:
		return "tool call denied by approval policy"
	}
}

func approvalPromptReason(cause approval.PromptCause) string {
	switch cause {
	case approval.PromptCauseNonMutating:
		return "Reads data without changing the workspace."
	case approval.PromptCauseWorkspaceWrite:
		return "Modifies files in the workspace."
	case approval.PromptCauseWorkspaceCommand:
		return "Runs commands in the workspace."
	case approval.PromptCauseNetworkAccess:
		return "Accesses network resources."
	case approval.PromptCauseOutsideWorkspace:
		return "Targets a path outside the workspace directory."
	case approval.PromptCauseUnknownMutation:
		return "Has filesystem mutation targets that could not be verified."
	case approval.PromptCauseCatastrophicCommand:
		return "Runs a high-confidence catastrophic shell command."
	default:
		return "Has an unknown safety classification."
	}
}

func denialReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "tool call denied by user"
	}
	return strings.TrimSpace(reason)
}

var _ InteractionToolAuthorizer = (*ToolAuthorizer)(nil)
