package plan

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/exactint"
)

func TestValidateSteps(t *testing.T) {
	tests := []struct {
		name    string
		steps   []Step
		wantErr bool
	}{
		{name: "empty clears plan"},
		{name: "all pending", steps: []Step{{Description: "inspect", Status: StatusPending}, {Description: "implement", Status: StatusPending}}},
		{name: "one in progress", steps: []Step{{Description: "inspect", Status: StatusCompleted}, {Description: "implement", Status: StatusInProgress}}},
		{name: "multiple completed", steps: []Step{{Description: "inspect", Status: StatusCompleted}, {Description: "implement", Status: StatusCompleted}}},
		{name: "missing description", steps: []Step{{Status: StatusPending}}, wantErr: true},
		{name: "blank description", steps: []Step{{Description: "  ", Status: StatusPending}}, wantErr: true},
		{name: "unknown status", steps: []Step{{Description: "inspect", Status: "done"}}, wantErr: true},
		{name: "two in progress", steps: []Step{{Description: "inspect", Status: StatusInProgress}, {Description: "implement", Status: StatusInProgress}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSteps(test.steps)
			if test.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateSteps error = %v, want ErrInvalid", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("ValidateSteps error = %v", err)
			}
		})
	}
}

func TestStateReplaceOwnsRevisionTimeAndClear(t *testing.T) {
	firstTime := time.Date(2026, 8, 10, 1, 2, 3, 4, time.FixedZone("offset", 8*60*60))
	input := []Step{{Description: "inspect", Status: StatusInProgress}}
	first, err := (Current{}).Replace(input, firstTime)
	if err != nil {
		t.Fatalf("first Replace: %v", err)
	}
	input[0].Description = "mutated"
	if first.Revision() != 1 || !first.UpdatedAt().Equal(firstTime) || first.UpdatedAt().Location() != time.UTC {
		t.Fatalf("first state = %+v", first.Snapshot())
	}
	if got := first.Steps(); len(got) != 1 || got[0].Description != "inspect" {
		t.Fatalf("Steps = %+v, retained caller input", got)
	}

	cleared, err := first.Replace(nil, firstTime.Add(time.Second))
	if err != nil {
		t.Fatalf("clear Replace: %v", err)
	}
	if cleared.Revision() != 2 || len(cleared.Steps()) != 0 {
		t.Fatalf("cleared state = %+v", cleared.Snapshot())
	}
}

func TestStateDefensiveCopies(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	snapshot := Snapshot{Steps: []Step{{Description: "original", Status: StatusPending}}, Revision: 4, UpdatedAt: now}
	state, err := Restore(snapshot)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	snapshot.Steps[0].Description = "changed input"
	steps := state.Steps()
	steps[0].Description = "changed output"
	persisted := state.Snapshot()
	persisted.Steps[0].Description = "changed snapshot"
	if got := state.Steps()[0].Description; got != "original" {
		t.Fatalf("state description = %q, want original", got)
	}
}

func TestRestoreRejectsImpossibleState(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name     string
		snapshot Snapshot
	}{
		{name: "steps without revision", snapshot: Snapshot{Steps: []Step{{Description: "x", Status: StatusPending}}}},
		{name: "time without revision", snapshot: Snapshot{UpdatedAt: now}},
		{name: "revision without time", snapshot: Snapshot{Revision: 1}},
		{name: "invalid steps", snapshot: Snapshot{Steps: []Step{{Description: "x", Status: "unknown"}}, Revision: 1, UpdatedAt: now}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Restore(test.snapshot); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Restore error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCurrentDistinguishesUnwrittenFromCommittedClear(t *testing.T) {
	unwritten := Current{}
	if _, committed := unwritten.State(); committed || !unwritten.Version().IsUnwritten() {
		t.Fatal("zero Current did not remain explicitly unwritten")
	}
	cleared, err := unwritten.Replace(nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	current, err := CurrentOf(cleared)
	if err != nil {
		t.Fatal(err)
	}
	if _, committed := current.State(); !committed || current.Version().IsUnwritten() {
		t.Fatal("committed empty Plan collapsed back to unwritten")
	}
}

func TestCurrentAndVersionRejectContradictoryState(t *testing.T) {
	now := time.Now().UTC()

	if _, err := CurrentOf(State{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CurrentOf zero State error = %v, want ErrInvalid", err)
	}

	for _, test := range []struct {
		name    string
		version Version
	}{
		{name: "committed without revision", version: Version{committed: true}},
		{name: "unwritten with revision", version: Version{revision: exactint.First()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.version.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate error = %v, want ErrInvalid", err)
			}
			valid, restoreErr := Restore(Snapshot{Revision: 1, UpdatedAt: now})
			if restoreErr != nil {
				t.Fatal(restoreErr)
			}
			if err := test.version.AdvancesTo(valid); !errors.Is(err, ErrInvalid) {
				t.Fatalf("AdvancesTo error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestReplaceRejectsTimeTravelAndRevisionOverflow(t *testing.T) {
	now := time.Now().UTC()
	state, err := Restore(Snapshot{Revision: 7, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, replaceErr := state.Replace(nil, now.Add(-time.Nanosecond)); !errors.Is(replaceErr, ErrInvalid) {
		t.Fatalf("time-travel error = %v, want ErrInvalid", replaceErr)
	}
	overflow, err := Restore(Snapshot{Revision: exactint.Maximum, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := overflow.Replace(nil, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overflow error = %v, want ErrInvalid", err)
	}
	if _, err := Restore(Snapshot{Revision: exactint.Maximum + 1, UpdatedAt: now}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("inexact restore error = %v, want ErrInvalid", err)
	}
}
