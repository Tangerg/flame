package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type identityApprovalPolicy struct {
	remembered  []approval.RememberRequest
	rememberErr error
}

func (*identityApprovalPolicy) Mode(context.Context, string) (approval.Mode, error) {
	return approval.ModeSafe, nil
}
func (*identityApprovalPolicy) Decide(context.Context, approval.Query) (approval.Decision, bool, error) {
	return "", false, nil
}
func (p *identityApprovalPolicy) Remember(_ context.Context, request approval.RememberRequest) error {
	if p.rememberErr != nil {
		return p.rememberErr
	}
	p.remembered = append(p.remembered, request)
	return nil
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
	request := ToolAuthorizationRequest{SessionID: "ses_1", WorkspaceCWD: "/workspace", CallID: "call_1", Tool: original, SourceFingerprint: testsupport.ToolFingerprint(original), ToolName: original.ModelName(), Arguments: args, SafetyClass: tool.SafetyClassNetwork, FileMutation: tool.FileMutationNone}
	policy := &identityApprovalPolicy{}
	authorizer, err := NewToolAuthorizer(policy, toolset.NewInterpreter(nil))
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
	answer := interrupt.Resolution{Approved: true, RememberScope: approval.ScopeGlobal}
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: request.ToolName}, func(context.Context, struct{}) (string, error) {
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
		{"different name", func(r *ToolAuthorizationRequest) { r.ToolName = "replacement" }},
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
			invalidAnswer := answer
			invalidAnswer.Arguments = `{"unexpected":true}`
			_, _, _, err := observed.resolveToolApproval(t.Context(), current, *restored.Approval, invalidAnswer)
			if _, known := errors.AsType[*toolcontract.Failure](err); err == nil || known {
				t.Fatalf("identity mismatch became an ordinary argument failure: %v", err)
			}
		})
	}
	if len(policy.remembered) != 0 {
		t.Fatal("mismatched response persisted a grant")
	}
	if _, err := authorizer.ResolveToolApproval(t.Context(), request, *restored.Approval, answer); err != nil {
		t.Fatal(err)
	}
	if len(policy.remembered) != 1 {
		t.Fatal("matching restored response did not remember decision")
	}
}

func TestRememberedApprovalUsesConfirmedArguments(t *testing.T) {
	for _, test := range []struct {
		name     string
		approved bool
		edited   string
		want     string
		invalid  bool
	}{
		{name: "edited command", approved: true, edited: `{"command":"echo approved"}`, want: "echo approved"},
		{name: "literal wildcard", approved: true, edited: `{"command":"echo *"}`, want: "echo *"},
		{name: "literal question mark", approved: true, edited: `{"command":"echo ?"}`, want: "echo ?"},
		{name: "literal bracket", approved: true, edited: `{"command":"echo ["}`, want: "echo ["},
		{name: "unchanged command", approved: true, want: "echo original"},
		{name: "denied command", want: "echo original"},
		{name: "invalid command", approved: true, edited: `{"command":false}`, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ref := testsupport.BuiltInTool(t, tool.Shell)
			arguments, err := tool.ParseArguments(`{"command":"echo original"}`)
			if err != nil {
				t.Fatal(err)
			}
			request := ToolAuthorizationRequest{
				SessionID: "ses_1", WorkspaceCWD: "/workspace", CallID: "call_1",
				Tool: ref, ToolName: ref.ModelName(), Arguments: arguments,
				SafetyClass: tool.SafetyClassExec, FileMutation: tool.FileMutationNone,
			}
			policy := &identityApprovalPolicy{}
			authorizer, err := NewToolAuthorizer(policy, toolset.NewInterpreter(nil))
			if err != nil {
				t.Fatal(err)
			}
			decision, err := authorizer.AuthorizeTool(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			prompt, found := decision.Approval()
			if !found {
				t.Fatal("expected approval")
			}
			encoded, err := runinput.EncodePrompt(runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt})
			if err != nil {
				t.Fatal(err)
			}
			restored, err := runinput.DecodePrompt(encoded)
			if err != nil {
				t.Fatal(err)
			}
			decision, err = authorizer.ResolveToolApproval(t.Context(), request, *restored.Approval, interrupt.Resolution{
				Approved: test.approved, Arguments: test.edited, RememberScope: approval.ScopeGlobal,
			})
			if test.invalid {
				if err == nil || len(policy.remembered) != 0 {
					t.Fatalf("invalid approved command: error=%v remembered=%+v", err, policy.remembered)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, denied := decision.Denied(); denied == test.approved {
				t.Fatalf("denied=%v approved=%v", denied, test.approved)
			}
			if len(policy.remembered) != 1 {
				t.Fatalf("remembered=%+v", policy.remembered)
			}
			rule, err := policy.remembered[0].Rule()
			if err != nil {
				t.Fatal(err)
			}
			for _, subject := range []string{test.want, "echo original", "echo approved", "echo x"} {
				verdict, matched, err := approval.Decide([]approval.Rule{rule}, approval.Query{Tool: ref, Subject: subject})
				if err != nil || matched != (subject == test.want) || matched && verdict != approval.DecisionOf(test.approved) {
					t.Fatalf("subject %q: verdict=%q matched=%v error=%v rule=%+v", subject, verdict, matched, err, rule)
				}
			}
		})
	}
}

func TestApprovalSourceChangeKeepsOneShotDecision(t *testing.T) {
	storeFailure := errors.New("store unavailable")
	for _, rememberErr := range []error{approval.ErrSourceAuthorityChanged, storeFailure} {
		for _, approved := range []bool{false, true} {
			ref := testsupport.BuiltInTool(t, tool.Shell)
			arguments, err := tool.ParseArguments(`{"command":"echo original"}`)
			if err != nil {
				t.Fatal(err)
			}
			request := ToolAuthorizationRequest{SessionID: "ses_1", WorkspaceCWD: "/workspace", CallID: "call_1", Tool: ref, ToolName: ref.ModelName(), Arguments: arguments, SafetyClass: tool.SafetyClassExec, FileMutation: tool.FileMutationNone}
			policy := &identityApprovalPolicy{rememberErr: rememberErr}
			authorizer, err := NewToolAuthorizer(policy, toolset.NewInterpreter(nil))
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
			decision, err = authorizer.ResolveToolApproval(t.Context(), request, prompt, interrupt.Resolution{Approved: approved, RememberScope: approval.ScopeGlobal})
			if rememberErr == storeFailure {
				if !errors.Is(err, storeFailure) {
					t.Fatalf("unexpected store error: %v", err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("changed source failed one-shot approval: %v", err)
			}
			if _, denied := decision.Denied(); denied == approved {
				t.Fatalf("approved=%v, denied=%v", approved, denied)
			}
			if len(policy.remembered) != 0 {
				t.Fatal("changed source persisted a rule")
			}
		}
	}
}
