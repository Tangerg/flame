package run

import (
	"testing"
)

var allStates = []State{Running, Waiting, Finished}

var allOutcomes = []Outcome{
	OutcomeCompleted,
	OutcomeCanceled,
	OutcomeTimedOut,
	OutcomeFailed,
	OutcomeLost,
}

func TestIsTerminal(t *testing.T) {
	for _, s := range allStates {
		if got := s.IsTerminal(); got != (s == Finished) {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, got, s == Finished)
		}
	}
}

func TestSuspend(t *testing.T) {
	for _, s := range allStates {
		got, ok := s.Suspend()
		wantOK := s == Running
		wantState := s
		if wantOK {
			wantState = Waiting
		}
		if got != wantState || ok != wantOK {
			t.Errorf("%s.Suspend() = (%s,%v), want (%s,%v)", s, got, ok, wantState, wantOK)
		}
	}
}

func TestResume(t *testing.T) {
	for _, s := range allStates {
		got, ok := s.Resume()
		wantOK := s == Waiting
		wantState := s
		if wantOK {
			wantState = Running
		}
		if got != wantState || ok != wantOK {
			t.Errorf("%s.Resume() = (%s,%v), want (%s,%v)", s, got, ok, wantState, wantOK)
		}
	}
}

// Running terminates for any outcome; Waiting only via cancel.
func TestTerminate(t *testing.T) {
	for _, s := range allStates {
		for _, o := range allOutcomes {
			got, ok := s.Terminate(o)
			wantOK := s == Running || (s == Waiting && o == OutcomeCanceled)
			wantState := s
			if wantOK {
				wantState = Finished
			}
			if got != wantState || ok != wantOK {
				t.Errorf("%s.Terminate(%s) = (%s,%v), want (%s,%v)", s, o, got, ok, wantState, wantOK)
			}
		}
	}
}

func TestTerminateRejectsUnknownOutcome(t *testing.T) {
	if got, ok := Running.Terminate(Outcome("invalid")); ok || got != Running {
		t.Fatalf("Running.Terminate(unknown) = (%s, %v), want (running, false)", got, ok)
	}
}

func TestRecoverLost(t *testing.T) {
	for _, state := range allStates {
		got, ok := state.RecoverLost()
		wantOK := !state.IsTerminal()
		want := state
		if wantOK {
			want = Finished
		}
		if got != want || ok != wantOK {
			t.Errorf("%s.RecoverLost() = (%s, %v), want (%s, %v)", state, got, ok, want, wantOK)
		}
	}
}

func TestOutcomeStringRoundTrip(t *testing.T) {
	for _, outcome := range allOutcomes {
		parsed, ok := ParseOutcome(string(outcome))
		if !ok || parsed != outcome {
			t.Errorf("ParseOutcome(%q) = (%s, %v), want (%s, true)", outcome, parsed, ok, outcome)
		}
	}
	if _, ok := ParseOutcome("error"); ok {
		t.Fatal("ParseOutcome accepted removed error terminology")
	}
}

func TestStringsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range allStates {
		if seen[string(s)] {
			t.Errorf("duplicate state label %q", s)
		}
		seen[string(s)] = true
	}
	seen = map[string]bool{}
	for _, o := range allOutcomes {
		if seen[string(o)] {
			t.Errorf("duplicate outcome label %q", o)
		}
		seen[string(o)] = true
	}
}

// TestDurability pins the commit-before-publish predicate.
