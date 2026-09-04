package session

import (
	"errors"
	"testing"
	"time"
)

func TestReplacementOwnsInitialAndExactNextRevision(t *testing.T) {
	current := mustNew(t, Draft{
		ID: "ses_1", Title: "Before", Workspace: mustWorkspace(t, "/work"), StartedAt: time.Unix(1, 0),
	})
	initial, err := InitialReplacement(current)
	if err != nil {
		t.Fatalf("InitialReplacement: %v", err)
	}
	if initial.ExpectedRevision() != 0 || initial.State().ID() != current.ID() {
		t.Fatalf("initial replacement = %+v", initial)
	}

	title := "After"
	next, changed, err := current.Apply(Patch{Title: &title}, time.Unix(2, 0))
	if err != nil || !changed {
		t.Fatalf("Apply: changed=%t err=%v", changed, err)
	}
	replacement, err := NextReplacement(current, next)
	if err != nil {
		t.Fatalf("NextReplacement: %v", err)
	}
	if replacement.ExpectedRevision() != current.Revision() || replacement.State().Title() != title {
		t.Fatalf("next replacement = %+v", replacement)
	}
}

func TestReplacementRejectsInvalidRevisionOrIdentity(t *testing.T) {
	current := mustNew(t, Draft{
		ID: "ses_1", Workspace: mustWorkspace(t, "/work"), StartedAt: time.Unix(1, 0),
	})
	title := "Second"
	second, _, err := current.Apply(Patch{Title: &title}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InitialReplacement(second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-initial replacement error = %v, want ErrInvalid", err)
	}

	other := mustNew(t, Draft{
		ID: "ses_2", Workspace: mustWorkspace(t, "/work"), StartedAt: time.Unix(1, 0),
	})
	otherTitle := "Other"
	otherNext, _, err := other.Apply(Patch{Title: &otherTitle}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NextReplacement(current, otherNext); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-identity replacement error = %v, want ErrInvalid", err)
	}
	if _, err := NextReplacement(Session{}, second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid expected replacement error = %v, want ErrInvalid", err)
	}
	older, err := Restore(Snapshot{
		ID: "ses_1", Workspace: mustWorkspace(t, "/work"), Selection: second.Selection(),
		StartedAt: second.StartedAt(), UpdatedAt: second.StartedAt(), Revision: second.Revision() + 1,
	})
	if err != nil {
		t.Fatalf("restore older replacement fixture: %v", err)
	}
	if _, err := NextReplacement(second, older); !errors.Is(err, ErrInvalid) {
		t.Fatalf("time-regressing replacement error = %v, want ErrInvalid", err)
	}
}
