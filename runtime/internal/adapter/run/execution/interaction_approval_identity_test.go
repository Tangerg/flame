package execution

import (
	"context"
	"errors"
	"testing"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// identityApprovalPolicy is a Mode+Decide approval stub. Remembering a decision
// is owned by the acceptance transaction, not the Tool boundary, so the policy a
// Tool consults no longer persists rules.
type identityApprovalPolicy struct{}

func (identityApprovalPolicy) Mode(context.Context, string) (approval.Mode, error) {
	return approval.ModeSafe, nil
}
func (identityApprovalPolicy) Decide(context.Context, approval.Query) (approval.Decision, bool, error) {
	return "", false, nil
}

func TestRestoredToolApprovalRejectsIdentityAndAuthorityDrift(t *testing.T) {
	ref := func(serverName, remoteName string) tool.Ref {
		t.Helper()
		return testsupport.MCPTool(testsupport.UserMCPServer(serverName), remoteName)
	}
	original := ref("a_b", "c")
	args, err := tool.ParseArguments(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	request := ToolAuthorizationRequest{SessionID: "ses_1", WorkspaceCWD: "/workspace", CallID: "call_1", Tool: original, SourceFingerprint: testsupport.ToolFingerprint(original), Arguments: args, SafetyClass: tool.SafetyClassNetwork, FileMutation: tool.FileMutationNone}
	authorizer, err := NewToolAuthorizer(identityApprovalPolicy{}, toolset.NewInterpreter(nil))
	if err != nil {
		t.Fatal(err)
	}
	decision, err := authorizer.AuthorizeTool(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	prompt, found := decision.Approval()
	if !found {
		t.Fatal("expected pending approval")
	}
	encoded, err := runinput.EncodePrompt(runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := runinput.DecodePrompt(encoded)
	if err != nil {
		t.Fatal(err)
	}
	answer := testsupport.ApprovalAnswer(t, approval.Resolution{Decision: approval.DecisionOf(true), RememberScope: approval.ScopeGlobal})
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: request.Tool.ModelName()}, func(context.Context, struct{}) (string, error) {
		return "unused", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		name   string
		mutate func(*ToolAuthorizationRequest)
	}{
		{"same label, different source", func(r *ToolAuthorizationRequest) { r.Tool = ref("a", "b_c") }},
		{"same name, different origin", func(r *ToolAuthorizationRequest) {
			r.Tool = testsupport.MCPTool(testsupport.InstallationMCPServer(testsupport.InstallationID(t), "a_b"), "c")
		}},
		{"different authority", func(r *ToolAuthorizationRequest) {
			r.SourceFingerprint = testsupport.Digest("different authority")
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			current := request
			change.mutate(&current)
			if _, err := authorizer.ResolveToolApproval(t.Context(), current, *restored.Approval, answer); err == nil {
				t.Fatal("restored response authorized a different invocation")
			}
			observed := observedInteractionTool{
				ref: current.Tool, binding: binding, interpreter: toolset.NewInterpreter(nil), authorizer: authorizer,
			}
			value := *answer.Approval()
			value.EditedArguments = `{"unexpected":true}`
			invalidAnswer := testsupport.ApprovalAnswer(t, value)
			_, _, _, err := observed.resolveToolApproval(t.Context(), current, *restored.Approval, invalidAnswer)
			if _, known := errors.AsType[*toolcontract.Failure](err); err == nil || known {
				t.Fatalf("identity mismatch became an ordinary argument failure: %v", err)
			}
		})
	}
	// The matching invocation still resolves without error; persisting a standing
	// rule for it is the acceptance owner's responsibility, not the Tool boundary's.
	if _, err := authorizer.ResolveToolApproval(t.Context(), request, *restored.Approval, answer); err != nil {
		t.Fatal(err)
	}
}
