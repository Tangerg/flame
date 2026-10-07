package terminal

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

// The picker describes a mode from the gates the Runtime publishes, so a policy
// the CLI has never seen reads exactly as published.
func TestApprovalPolicyDetailFollowsThePublishedGates(t *testing.T) {
	policy := protocol.ApprovalModePolicy{
		Mode: protocol.ApprovalModeBalanced, Write: protocol.ApprovalGatePass,
		Exec: protocol.ApprovalGateDeny, Network: protocol.ApprovalGatePrompt,
	}
	want := "write allowed · exec refused · network asks first"
	if got := approvalPolicyDetail(policy); got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
}
