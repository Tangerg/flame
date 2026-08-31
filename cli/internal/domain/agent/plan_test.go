package agent

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/exactint"
)

func TestPlanCouplesRevisionAndOwnedContent(t *testing.T) {
	items := []PlanItem{{Title: "inspect", Status: PlanActive}}
	plan, err := NewPlan(7, items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].Title = "mutated input"
	projected := plan.Items()
	projected[0].Title = "mutated output"

	if plan.Revision() != 7 || plan.Items()[0].Title != "inspect" {
		t.Fatalf("Plan leaked mutable content: revision=%d items=%+v", plan.Revision(), plan.Items())
	}
}

func TestPlanDistinguishesAbsenceFromCommittedClear(t *testing.T) {
	content, err := NewPlanContent(nil)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := CommitNextPlan(nil, content)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Revision() != 1 || cleared.Items() == nil || len(cleared.Items()) != 0 {
		t.Fatalf("committed clear = revision %d items %#v", cleared.Revision(), cleared.Items())
	}
}

func TestPlanRevisionCannotWrapOnLongLivedSession(t *testing.T) {
	content, err := NewPlanContent(nil)
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := CommitPlan(exactint.Maximum, content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitNextPlan(&exhausted, content); err == nil {
		t.Fatal("CommitNextPlan accepted revision overflow")
	}
	if _, err := CommitPlan(exactint.Maximum+1, content); err == nil {
		t.Fatal("CommitPlan accepted an inexact JSON revision")
	}
}
