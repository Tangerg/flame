package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type planStoreFake struct {
	state           plan.Current
	expectedVersion plan.Version
	saved           *plan.State
	readErr         error
	saveErr         error
}

func TestNewPlanCoordinatorRequiresStore(t *testing.T) {
	var typedNil *planStoreFake
	for _, store := range []PlanStore{nil, typedNil} {
		if coordinator, err := NewPlanCoordinator(PlanDependencies{Store: store}); err == nil || coordinator != nil {
			t.Fatalf("NewPlanCoordinator = (%v, %v), want missing store error", coordinator, err)
		}
	}
}

func (f *planStoreFake) State(context.Context, string) (plan.Current, error) {
	return f.state, f.readErr
}
func (f *planStoreFake) Save(_ context.Context, _ string, replacement plan.Replacement) error {
	f.expectedVersion = replacement.ExpectedVersion()
	owned := replacement.State()
	f.saved = &owned
	return f.saveErr
}

// TestCommittedPlanChangeReachesOtherWindows proves
// committed_plan_change_reaches_other_windows at its mutation owner: only a
// successful CAS publishes a session-scoped Plan invalidation.
func TestCommittedPlanChangeReachesOtherWindows(t *testing.T) {
	now := time.Date(2026, 8, 10, 2, 3, 4, 0, time.UTC)
	current, err := plan.Restore(plan.Snapshot{Revision: 3, UpdatedAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	currentProjection, err := plan.CurrentOf(current)
	if err != nil {
		t.Fatal(err)
	}
	store := &planStoreFake{state: currentProjection}
	var notices []invalidation.Notice
	coordinator := mustPlanCoordinator(PlanDependencies{
		Store: store, Now: func() time.Time { return now },
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})
	got, err := coordinator.Replace(t.Context(), "ses_1", []plan.Step{{Description: "ship", Status: plan.StatusInProgress}})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	expectedRevision, committed := store.expectedVersion.Revision()
	if !committed || expectedRevision != 3 || store.saved == nil || store.saved.Revision() != 4 || got.Revision() != 4 || !got.UpdatedAt().Equal(now) {
		t.Fatalf("saved = %+v expected=%s got=%+v", store.saved, store.expectedVersion, got.Snapshot())
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.PlanState || len(notices[0].SessionIDs) != 1 || notices[0].SessionIDs[0] != "ses_1" {
		t.Fatalf("notices = %+v, want the committed Plan state", notices)
	}
}

func TestPrepareReplacementDoesNotWrite(t *testing.T) {
	store := &planStoreFake{}
	coordinator := mustPlanCoordinator(PlanDependencies{Store: store, Now: func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) }})
	replacement, err := coordinator.PrepareReplacement(t.Context(), "ses_1", nil)
	if err != nil {
		t.Fatalf("PrepareReplacement: %v", err)
	}
	if !replacement.ExpectedVersion().IsUnwritten() || replacement.State().Revision() != 1 || store.saved != nil {
		t.Fatalf("replacement = expected %s state %+v; store was %+v", replacement.ExpectedVersion(), replacement.State().Snapshot(), store.saved)
	}
}

func TestPreparedPlanOwnsStepsAfterReturn(t *testing.T) {
	steps := []plan.Step{{Description: "original", Status: plan.StatusPending}}
	coordinator := mustPlanCoordinator(PlanDependencies{Store: &planStoreFake{}, Now: time.Now})
	replacement, err := coordinator.PrepareReplacement(t.Context(), "ses_1", steps)
	if err != nil {
		t.Fatal(err)
	}
	steps[0].Description = "changed"
	projected := replacement.State().Steps()
	projected[0].Description = "changed projection"
	if got := replacement.State().Steps()[0].Description; got != "original" {
		t.Fatalf("prepared step = %q, want original", got)
	}
}

func TestReplacePropagatesRevisionConflict(t *testing.T) {
	store := &planStoreFake{saveErr: plan.ErrRevisionConflict}
	var published bool
	coordinator := mustPlanCoordinator(PlanDependencies{
		Store: store, Now: time.Now,
		Invalidations: func(invalidation.Notice) { published = true },
	})
	_, err := coordinator.Replace(t.Context(), "ses_1", nil)
	if !errors.Is(err, plan.ErrRevisionConflict) {
		t.Fatalf("Replace error = %v, want ErrRevisionConflict", err)
	}
	if published {
		t.Fatal("failed replacement published a Plan change")
	}
}

func TestStateRejectsInvalidSessionIdentityBeforePersistence(t *testing.T) {
	store := &planStoreFake{}
	coordinator := mustPlanCoordinator(PlanDependencies{Store: store, Now: time.Now})
	for _, sessionID := range []string{"", " ses_1", "ses_1 "} {
		if _, err := coordinator.State(t.Context(), sessionID); err == nil {
			t.Errorf("State(%q) succeeded", sessionID)
		}
	}
}

func mustPlanCoordinator(deps PlanDependencies) *PlanCoordinator {
	coordinator, err := NewPlanCoordinator(deps)
	if err != nil {
		panic(err)
	}
	return coordinator
}
