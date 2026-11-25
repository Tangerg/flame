package bootstrap

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestApprovalModeAndItsReceiptCommitTogether(t *testing.T) {
	const setMode = delivery.Name("approval.setMode")
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	ctx := protocolLifecycleContext(t.Context())
	original, api := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := original.Close(); err != nil {
			t.Error(err)
		}
	})
	initial := protocol.SetApprovalModeRequest{Mode: protocol.ApprovalModeBalanced}
	if _, err := api.SetApprovalMode(ctx, initial); err != nil {
		t.Fatal(err)
	}
	allowReceipt := refuseApprovalReceipt(t, home)
	request := protocol.SetApprovalModeRequest{Mode: protocol.ApprovalModeSafe}
	meta := protocol.CallMetadata{IdempotencyKey: "original-approval-mode"}
	if refused, err := original.Endpoint().Call[protocol.SetApprovalModeRequest, *protocol.ApprovalModeResult](ctx, setMode, request, meta); refused != nil || err == nil {
		t.Fatalf("mode write with a refused receipt = %+v, %v", refused, err)
	}
	if current, err := api.GetApprovalMode(ctx); err != nil || current.Mode != initial.Mode {
		t.Fatalf("approval mode after rejected transaction = %+v, %v; want %s", current, err, initial.Mode)
	}
	allowReceipt()
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, api := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	setOriginal := func() {
		t.Helper()
		result, err := restarted.Endpoint().Call[protocol.SetApprovalModeRequest, *protocol.ApprovalModeResult](ctx, setMode, request, meta)
		if err != nil || result.Mode != request.Mode {
			t.Fatalf("original keyed mode = %+v, %v; want %s", result, err, request.Mode)
		}
	}
	setOriginal()
	if _, err := api.SetApprovalMode(ctx, initial); err != nil {
		t.Fatal(err)
	}
	setOriginal()
	if current, err := api.GetApprovalMode(ctx); err != nil || current.Mode != initial.Mode {
		t.Fatalf("replay changed the newer mode = %+v, %v; want %s", current, err, initial.Mode)
	}
}

func TestApprovalRuleCreationAndRemovalCommitWithTheirReceipts(t *testing.T) {
	const (
		setRule    = delivery.Name("approval.setRule")
		forgetRule = delivery.Name("approval.forgetRule")
	)
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)
	ctx := protocolLifecycleContext(t.Context())
	host, api := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	rule := protocol.SetApprovalRuleRequest{
		Scope:    protocol.ApprovalRuleScopeGlobal,
		Tool:     protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: string(builtin.Read)},
		Subject:  protocol.ApprovalSubject{Type: protocol.ApprovalSubjectExact, Value: "reviewed-notes.txt"},
		Decision: protocol.ApprovalRuleDecisionDeny,
	}
	rules := func(count int) []protocol.ApprovalRule {
		t.Helper()
		result, err := api.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{})
		if err != nil || len(result.Rules) != count {
			t.Fatalf("approval rules = %+v, %v; want %d", result, err, count)
		}
		return result.Rules
	}
	add := func() protocol.OperationResult {
		return host.Endpoint().Invoke(ctx, setRule, rule, protocol.CallMetadata{IdempotencyKey: "original-rule-addition"})
	}
	allowReceipt := refuseApprovalReceipt(t, home)
	if result := add(); result.Failure == nil {
		t.Fatal("added a rule despite a refused receipt")
	}
	rules(0)
	allowReceipt()
	if result := add(); result.Failure != nil {
		t.Fatal(result.Failure)
	}
	forget := protocol.ForgetApprovalRuleRequest{ID: rules(1)[0].ID}
	remove := func() protocol.OperationResult {
		return host.Endpoint().Invoke(ctx, forgetRule, forget, protocol.CallMetadata{IdempotencyKey: "original-rule-removal"})
	}
	allowReceipt = refuseApprovalReceipt(t, home)
	if result := remove(); result.Failure == nil {
		t.Fatal("removed a rule despite a refused receipt")
	}
	rules(1)
	allowReceipt()
	if result := remove(); result.Failure != nil {
		t.Fatal(result.Failure)
	}
	if result := add(); result.Failure != nil {
		t.Fatal(result.Failure)
	}
	rules(0)
	if err := api.SetApprovalRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if result := remove(); result.Failure != nil {
		t.Fatal(result.Failure)
	}
	rules(1)
}

func refuseApprovalReceipt(t *testing.T, home string) func() {
	t.Helper()
	questionCrashDatabase(t, home, `CREATE TRIGGER refuse_approval_ack
		BEFORE UPDATE OF payload ON idempotency_records
		BEGIN SELECT RAISE(ABORT, 'approval receipt refused'); END`)
	return func() {
		t.Helper()
		questionCrashDatabase(t, home, `DROP TRIGGER refuse_approval_ack`)
	}
}
