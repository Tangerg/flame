package execution

import (
	"context"
	"errors"
	"testing"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type ruleBackedApprovalPolicy struct {
	mode    approval.Mode
	rules   []approval.Rule
	queries []approval.Query
	err     error
}

func (p *ruleBackedApprovalPolicy) Mode(context.Context, string) (approval.Mode, error) {
	return p.mode, nil
}

func (p *ruleBackedApprovalPolicy) Decide(_ context.Context, query approval.Query) (approval.Decision, bool, error) {
	p.queries = append(p.queries, query)
	if p.err != nil {
		return "", false, p.err
	}
	return approval.Decide(p.rules, query)
}

func TestToolAuthorizerKeepsSavedDenialWhenModeChanges(t *testing.T) {
	ref := testsupport.MCPTool(testsupport.UserMCPServer("documents"), "update")
	arguments, err := tool.ParseArguments(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := approval.NewRule(approval.GlobalScope(), ref, testsupport.ToolFingerprint(ref), approval.InvocationSubject(""), approval.Deny)
	if err != nil {
		t.Fatal(err)
	}
	policy := &ruleBackedApprovalPolicy{rules: []approval.Rule{rule}}
	authorizer, err := NewToolAuthorizer(policy, toolset.NewInterpreter(nil))
	if err != nil {
		t.Fatal(err)
	}
	request := ToolAuthorizationRequest{
		SessionID: "session", WorkspaceCWD: t.TempDir(), CallID: "call",
		Tool: ref, SourceFingerprint: testsupport.ToolFingerprint(ref), Arguments: arguments,
		SafetyClass: tool.SafetyClassWrite, FileMutation: tool.FileMutationNone,
	}
	modes := []approval.Mode{approval.ModeSafe, approval.ModeBalanced, approval.ModeYolo}
	for _, mode := range modes {
		policy.mode = mode
		decision, err := authorizer.AuthorizeTool(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if _, denied := decision.Denied(); !denied {
			t.Fatalf("mode %q bypassed the saved denial", mode)
		}
		if _, pending := decision.Approval(); pending {
			t.Fatalf("mode %q asks to override a saved denial", mode)
		}
	}
	if len(policy.queries) != len(modes) {
		t.Fatalf("remembered rules evaluated %d times, want all three modes", len(policy.queries))
	}
	for _, query := range policy.queries {
		if query.SessionID != request.SessionID || query.ProjectDir != request.WorkspaceCWD || query.Tool != ref || query.SourceFingerprint != request.SourceFingerprint {
			t.Fatalf("rule lookup lost invocation authority: %+v", query)
		}
	}
}

func TestToolAuthorizerChecksRulesForReadOnlyCalls(t *testing.T) {
	ref := testsupport.BuiltInTool(t, builtincontract.Read)
	arguments, err := tool.ParseArguments(`{"path":"private.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := approval.NewRule(approval.GlobalScope(), ref, testsupport.ToolFingerprint(ref), approval.InvocationSubject("private.txt"), approval.Deny)
	if err != nil {
		t.Fatal(err)
	}
	policy := &ruleBackedApprovalPolicy{mode: approval.ModePlan, rules: []approval.Rule{rule}}
	authorizer, err := NewToolAuthorizer(policy, toolset.NewInterpreter(nil))
	if err != nil {
		t.Fatal(err)
	}
	request := ToolAuthorizationRequest{
		SessionID: "session", WorkspaceCWD: t.TempDir(), CallID: "call",
		Tool: ref, SourceFingerprint: testsupport.ToolFingerprint(ref), Arguments: arguments,
		SafetyClass: tool.SafetyClassSafe, FileMutation: tool.FileMutationNone,
	}
	decision, err := authorizer.AuthorizeTool(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, denied := decision.Denied(); !denied {
		t.Fatal("read-only mode bypassed an exact path denial")
	}
	request.Arguments, err = tool.ParseArguments(`{"path":"public.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	decision, err = authorizer.AuthorizeTool(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, denied := decision.Denied(); denied {
		t.Fatal("an exact path denial widened to another path")
	}
	policy.err = errors.New("rule store unavailable")
	if _, err := authorizer.AuthorizeTool(t.Context(), request); !errors.Is(err, policy.err) {
		t.Fatalf("policy failure was auto-approved: %v", err)
	}
}
