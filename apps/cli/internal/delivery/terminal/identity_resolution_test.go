package terminal

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

// TestResolveByIDPrefersExactOverPrefix pins the rule four resolvers used to
// carry a copy of each. When one id prefixes another, typing the shorter one in
// full names it — dropping the exact pass would make that ambiguous instead.
func TestResolveByIDPrefersExactOverPrefix(t *testing.T) {
	t.Parallel()
	rules := []protocol.ApprovalRule{{ID: "rule_1"}, {ID: "rule_12"}}

	exact, err := resolveApprovalRule(rules, "rule_1")
	if err != nil || exact.ID != "rule_1" {
		t.Fatalf("exact id = %+v, %v; want rule_1", exact, err)
	}
	unique, err := resolveApprovalRule(rules, "rule_12")
	if err != nil || unique.ID != "rule_12" {
		t.Fatalf("unique prefix = %+v, %v; want rule_12", unique, err)
	}
	if _, err := resolveApprovalRule(rules, "rule_"); err == nil {
		t.Fatal("a prefix shared by two rules resolved instead of reporting ambiguity")
	}
	if _, err := resolveApprovalRule(rules, "absent"); err == nil {
		t.Fatal("an unknown id resolved")
	}
}
