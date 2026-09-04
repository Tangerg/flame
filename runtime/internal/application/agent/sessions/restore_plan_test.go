package sessions

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRestorePlanOwnsItsCommittedProjection(t *testing.T) {
	snapshot := portableSnapshot()
	snapshot.Messages = []chat.Message{chat.NewUserMessage(chat.NewTextPart("remembered"))}
	replacement, err := session.InitialReplacement(snapshot.Session)
	if err != nil {
		t.Fatal(err)
	}
	restore, err := NewRestorePlan(snapshot, replacement, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Messages[0].Parts[0].Text = "changed source"
	snapshot.Runs = nil

	owned := restore.Snapshot()
	if owned.Session.Snapshot() != replacement.State().Snapshot() || owned.Messages[0].Parts[0].Text != "remembered" || len(owned.Runs) != 1 {
		t.Fatalf("restore projection = %+v", owned)
	}
	owned.Messages[0].Parts[0].Text = "changed accessor"
	owned.Items = nil
	if got := restore.Snapshot(); got.Messages[0].Parts[0].Text != "remembered" || len(got.Items) != 1 {
		t.Fatalf("returned projection mutated restore plan: %+v", got)
	}
	if err := restore.Validate(); err != nil {
		t.Fatalf("owned restore plan became invalid: %v", err)
	}
}

func TestRestorePlanBindsTheExactPlanTransition(t *testing.T) {
	snapshot := portableSnapshot()
	steps := []plan.Step{{Description: "restored", Status: plan.StatusInProgress}}
	snapshot.Plan = steps
	planReplacement := initialForkPlanReplacement(t, steps)
	sessionReplacement, err := session.InitialReplacement(snapshot.Session)
	if err != nil {
		t.Fatal(err)
	}
	restore, err := NewRestorePlan(snapshot, sessionReplacement, &planReplacement)
	if err != nil {
		t.Fatal(err)
	}
	steps[0].Description = "changed source"
	if got := restore.Snapshot().Plan; len(got) != 1 || got[0].Description != "restored" {
		t.Fatalf("restored Plan = %+v", got)
	}
}

func TestRestorePlanRejectsIncoherentWriteSets(t *testing.T) {
	valid := portableSnapshot()
	sessionReplacement, err := session.InitialReplacement(valid.Session)
	if err != nil {
		t.Fatal(err)
	}
	other := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_other"})
	otherReplacement, err := session.InitialReplacement(other)
	if err != nil {
		t.Fatal(err)
	}
	steps := []plan.Step{{Description: "restored", Status: plan.StatusPending}}
	planReplacement := initialForkPlanReplacement(t, steps)

	invalidSnapshot := valid
	invalidSnapshot.Messages = []chat.Message{{}}
	withSteps := valid
	withSteps.Plan = steps
	mismatchedSteps := valid
	mismatchedSteps.Plan = []plan.Step{{Description: "different", Status: plan.StatusPending}}

	tests := []struct {
		name        string
		snapshot    Snapshot
		session     session.Replacement
		replacement *plan.Replacement
	}{
		{name: "invalid Session replacement", snapshot: valid, session: session.Replacement{}},
		{name: "different Session replacement", snapshot: valid, session: otherReplacement},
		{name: "invalid snapshot", snapshot: invalidSnapshot, session: sessionReplacement},
		{name: "missing Plan replacement", snapshot: withSteps, session: sessionReplacement},
		{name: "invalid Plan replacement", snapshot: withSteps, session: sessionReplacement, replacement: &plan.Replacement{}},
		{name: "different Plan replacement", snapshot: mismatchedSteps, session: sessionReplacement, replacement: &planReplacement},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRestorePlan(test.snapshot, test.session, test.replacement); err == nil {
				t.Fatal("NewRestorePlan accepted an incoherent write-set")
			}
		})
	}
	if err := (RestorePlan{}).Validate(); err == nil {
		t.Fatal("zero RestorePlan is valid")
	}
}
