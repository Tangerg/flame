package schedule

import (
	"errors"
	"testing"
	"time"
)

func TestReplacementProtectsManagementEditIdentityAndLifecycle(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	expected, err := New("sch_1", Draft{Instructions: "before", Cron: "@daily"}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	instructions := "after"
	state, err := expected.Edit(Patch{Instructions: &instructions}, expected.Revision(), createdAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := NewReplacement(expected, state)
	if err != nil {
		t.Fatalf("NewReplacement: %v", err)
	}
	if replacement.ExpectedRevision() != expected.Revision() || replacement.State().Instructions() != instructions {
		t.Fatalf("replacement = %+v", replacement)
	}

	other, err := New("sch_2", Draft{Instructions: "other", Cron: "@daily"}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	otherState, err := other.Edit(Patch{}, other.Revision(), createdAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReplacement(expected, otherState); err == nil {
		t.Fatal("NewReplacement accepted a different Schedule identity")
	}

	next, err := state.Edit(Patch{}, state.Revision(), createdAt.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReplacement(expected, next); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("skipped revision error = %v, want ErrRevisionConflict", err)
	}

	changedLifecycle := state.Snapshot()
	changedLifecycle.LastRunAt = createdAt.Add(time.Second)
	lifecycleState, err := Restore(changedLifecycle)
	if err != nil {
		t.Fatalf("restore changed lifecycle fixture: %v", err)
	}
	if _, err := NewReplacement(expected, lifecycleState); err == nil {
		t.Fatal("NewReplacement accepted a changed accepted-Run cursor")
	}
}
