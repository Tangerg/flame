package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestForkPlanOwnsTheCompleteChildProjection(t *testing.T) {
	parentID, snapshot := validForkPlanSnapshot(t)
	fork, err := NewForkPlan(parentID, snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Messages[0].Parts[0].Text = "changed source"
	snapshot.Plan = append(snapshot.Plan, plan.Step{Description: "changed source", Status: plan.StatusPending})

	owned := fork.Snapshot()
	if fork.ParentID() != parentID || fork.Child().ID() != snapshot.Session.ID() || owned.Messages[0].Parts[0].Text != "remembered" {
		t.Fatalf("fork ownership = parent:%q child:%q messages:%+v", fork.ParentID(), fork.Child().ID(), owned.Messages)
	}
	owned.Messages[0].Parts[0].Text = "changed accessor"
	owned.Runs = append(owned.Runs, owned.Runs...)
	if got := fork.Snapshot(); got.Messages[0].Parts[0].Text != "remembered" || len(got.Runs) != 0 {
		t.Fatalf("returned snapshot mutated fork plan: %+v", got)
	}
	if err := fork.Validate(); err != nil {
		t.Fatalf("owned fork plan became invalid: %v", err)
	}
}

func TestForkPlanBindsTheInitialPlanReplacement(t *testing.T) {
	parentID, snapshot := validForkPlanSnapshot(t)
	steps := []plan.Step{{Description: "inherited", Status: plan.StatusInProgress}}
	snapshot.Plan = steps
	replacement := initialForkPlanReplacement(t, steps)
	fork, err := NewForkPlan(parentID, snapshot, &replacement)
	if err != nil {
		t.Fatal(err)
	}
	steps[0].Description = "changed source"
	got := fork.PlanReplacement()
	if got == nil || got.State().Steps()[0].Description != "inherited" || fork.Snapshot().Plan[0].Description != "inherited" {
		t.Fatalf("fork Plan replacement = %+v", got)
	}
	if got == fork.PlanReplacement() {
		t.Fatal("fork returned its owned replacement pointer")
	}
}

func TestForkPlanRejectsIncoherentWriteSets(t *testing.T) {
	parentID, valid := validForkPlanSnapshot(t)
	steps := []plan.Step{{Description: "inherited", Status: plan.StatusPending}}
	replacement := initialForkPlanReplacement(t, steps)
	subsequent := subsequentForkPlanReplacement(t, steps)

	wrongRevision := valid
	childSnapshot := valid.Session.Snapshot()
	childSnapshot.Revision = 2
	wrongRevision.Session = testsupport.MustRestoreSession(childSnapshot)

	invalidSnapshot := valid
	invalidSnapshot.Messages = []chat.Message{{}}
	withSteps := valid
	withSteps.Plan = steps
	mismatchedSteps := valid
	mismatchedSteps.Plan = []plan.Step{{Description: "different", Status: plan.StatusPending}}

	tests := []struct {
		name        string
		parentID    string
		snapshot    Snapshot
		replacement *plan.Replacement
	}{
		{name: "invalid parent", parentID: "", snapshot: valid},
		{name: "different parent", parentID: "ses_other", snapshot: valid},
		{name: "non-initial child", parentID: parentID, snapshot: wrongRevision},
		{name: "invalid snapshot", parentID: parentID, snapshot: invalidSnapshot},
		{name: "missing Plan replacement", parentID: parentID, snapshot: withSteps},
		{name: "invalid Plan replacement", parentID: parentID, snapshot: withSteps, replacement: &plan.Replacement{}},
		{name: "non-initial Plan replacement", parentID: parentID, snapshot: withSteps, replacement: &subsequent},
		{name: "unexpected Plan replacement", parentID: parentID, snapshot: valid, replacement: &replacement},
		{name: "different Plan replacement", parentID: parentID, snapshot: mismatchedSteps, replacement: &replacement},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewForkPlan(test.parentID, test.snapshot, test.replacement); err == nil {
				t.Fatal("NewForkPlan accepted an incoherent write-set")
			}
		})
	}
	if err := (ForkPlan{}).Validate(); err == nil {
		t.Fatal("zero ForkPlan is valid")
	}
}

func validForkPlanSnapshot(t *testing.T) (string, Snapshot) {
	t.Helper()
	parent := testsupport.MustRestoreSession(session.Snapshot{
		ID: "ses_parent", Workspace: testsupport.MustWorkspace("/repo"),
	})
	child, err := parent.Fork("ses_child", "branch", time.Unix(10, 0))
	if err != nil {
		t.Fatalf("fork child Session: %v", err)
	}
	return parent.ID(), Snapshot{
		Session:  child,
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("remembered"))},
	}
}

func initialForkPlanReplacement(t *testing.T, steps []plan.Step) plan.Replacement {
	t.Helper()
	state, err := (plan.Current{}).Replace(steps, time.Unix(11, 0))
	if err != nil {
		t.Fatalf("prepare initial Plan state: %v", err)
	}
	replacement, err := plan.NewReplacement(plan.Version{}, state)
	if err != nil {
		t.Fatalf("prepare initial Plan replacement: %v", err)
	}
	return replacement
}

func subsequentForkPlanReplacement(t *testing.T, steps []plan.Step) plan.Replacement {
	t.Helper()
	initial := initialForkPlanReplacement(t, steps).State()
	current, err := plan.CurrentOf(initial)
	if err != nil {
		t.Fatalf("restore current Plan: %v", err)
	}
	next, err := current.Replace(steps, time.Unix(12, 0))
	if err != nil {
		t.Fatalf("prepare subsequent Plan state: %v", err)
	}
	replacement, err := plan.NewReplacement(current.Version(), next)
	if err != nil {
		t.Fatalf("prepare subsequent Plan replacement: %v", err)
	}
	return replacement
}
