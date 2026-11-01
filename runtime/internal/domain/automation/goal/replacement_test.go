package goal

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestReplacementOwnsOneExactGoalVersionAdvance(t *testing.T) {
	expected, err := Unwritten("ses_1")
	if err != nil {
		t.Fatal(err)
	}
	capabilities := run.Capabilities{InterruptTypes: []run.InterruptType{run.InterruptQuestion}}
	state := testGoalFor(t, "ses_1", "inc_1")
	state.capabilities = capabilities.Normalized()
	replacement, err := NewReplacement(expected.Version(), state)
	if err != nil {
		t.Fatalf("NewReplacement: %v", err)
	}
	state.capabilities.InterruptTypes[0] = run.InterruptApproval
	owned := replacement.State()
	read := owned.Capabilities()
	read.InterruptTypes[0] = run.InterruptApproval
	if !replacement.State().Capabilities().Equal(run.Capabilities{InterruptTypes: []run.InterruptType{run.InterruptQuestion}}) {
		t.Fatal("Replacement shares Goal capability storage")
	}
	if replacement.ExpectedVersion() != expected.Version() {
		t.Fatal("Replacement lost its exact unwritten version")
	}

	next, err := publishedFixture(state, state.UpdatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	tooFar, err := publishedFixture(next, next.UpdatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReplacement(state.Version(), tooFar); !errors.Is(err, ErrInvalid) {
		t.Fatalf("skipped revision error = %v, want ErrInvalid", err)
	}

	other := testGoalFor(t, "ses_2", "inc_2")
	if _, err := NewReplacement(state.Version(), other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-Session error = %v, want ErrInvalid", err)
	}
}
