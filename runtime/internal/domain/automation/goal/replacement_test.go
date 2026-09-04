package goal

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
)

func TestReplacementOwnsOneExactGoalVersionAdvance(t *testing.T) {
	expected, err := Unwritten("ses_1")
	if err != nil {
		t.Fatal(err)
	}
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	state := testGoalFor(t, "ses_1", "inc_1", UnlimitedBudget())
	state.capabilities = capabilities.Normalized()
	replacement, err := NewReplacement(expected.Version(), state)
	if err != nil {
		t.Fatalf("NewReplacement: %v", err)
	}
	state.capabilities.InterruptKinds[0] = interrupt.Approval
	owned := replacement.State()
	read := owned.Capabilities()
	read.InterruptKinds[0] = interrupt.Approval
	if !replacement.State().Capabilities().Equal(run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}) {
		t.Fatal("Replacement shares Goal capability storage")
	}
	if replacement.ExpectedVersion() != expected.Version() {
		t.Fatal("Replacement lost its exact unwritten version")
	}

	next, err := state.Pause(ReasonStoppedByUser, "", state.UpdatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	tooFar, err := next.Resume(next.UpdatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReplacement(state.Version(), tooFar); !errors.Is(err, ErrInvalid) {
		t.Fatalf("skipped revision error = %v, want ErrInvalid", err)
	}

	other := testGoalFor(t, "ses_2", "inc_2", UnlimitedBudget())
	if _, err := NewReplacement(state.Version(), other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-Session error = %v, want ErrInvalid", err)
	}
}
