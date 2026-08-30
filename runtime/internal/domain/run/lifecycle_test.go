package run

import (
	"math"
	"testing"
)

var allStates = []State{Running, Waiting, Completed, Failed, Canceled}

var allOutcomes = []Outcome{
	OutcomeCompleted,
	OutcomeCanceled,
	OutcomeTimedOut,
	OutcomeFailed,
	OutcomeMaxBudget,
	OutcomeMaxSteps,
	OutcomeLost,
}

// TestIsTerminal pins the terminal set: exactly Completed / Failed / Canceled.
func TestIsTerminal(t *testing.T) {
	terminal := map[State]bool{Completed: true, Failed: true, Canceled: true}
	for _, s := range allStates {
		if got := s.IsTerminal(); got != terminal[s] {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, got, terminal[s])
		}
	}
}

// TestSuspend: only Running parks (→ Waiting).
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

// TestResume: only Waiting continues (→ Running).
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

// TestTerminate is the full (state × outcome) matrix. Running terminates for any
// outcome; Waiting terminates only via cancel; all other states reject.
func TestTerminate(t *testing.T) {
	for _, s := range allStates {
		for _, o := range allOutcomes {
			got, ok := s.Terminate(o)
			var wantState State
			var wantOK bool
			switch {
			case s == Running:
				wantState, wantOK = o.terminalState(), true
			case s == Waiting && o == OutcomeCanceled:
				wantState, wantOK = Canceled, true
			default:
				wantState, wantOK = s, false
			}
			if got != wantState || ok != wantOK {
				t.Errorf("%s.Terminate(%s) = (%s,%v), want (%s,%v)", s, o, got, ok, wantState, wantOK)
			}
			if ok && !got.IsTerminal() {
				t.Errorf("%s.Terminate(%s) produced non-terminal %s", s, o, got)
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
		wantOK := state == Running || state == Waiting
		want := state
		if wantOK {
			want = Failed
		}
		if got != want || ok != wantOK {
			t.Errorf("%s.RecoverLost() = (%s, %v), want (%s, %v)", state, got, ok, want, wantOK)
		}
	}
}

func TestRunLimitsValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		values LimitValues
	}{
		{name: "empty"},
		{name: "negative tokens", values: LimitValues{MaxTotalTokens: int64Pointer(-1)}},
		{name: "negative steps", values: LimitValues{MaxSteps: intPointer(-1)}},
		{name: "negative budget", values: LimitValues{MaxBudgetUSD: floatPointer(-1)}},
		{name: "nan budget", values: LimitValues{MaxBudgetUSD: floatPointer(math.NaN())}},
		{name: "positive infinite budget", values: LimitValues{MaxBudgetUSD: floatPointer(math.Inf(1))}},
		{name: "negative infinite budget", values: LimitValues{MaxBudgetUSD: floatPointer(math.Inf(-1))}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewLimits(test.values); err == nil {
				t.Fatal("NewLimits accepted malformed limits")
			}
		})
	}
	if limits := UnlimitedLimits(); !limits.Unlimited() || limits.Validate() != nil {
		t.Fatalf("UnlimitedLimits = %+v", limits)
	}
	limited, err := NewLimits(LimitValues{
		MaxTotalTokens: int64Pointer(10), MaxSteps: intPointer(2), MaxBudgetUSD: floatPointer(0.25),
	})
	if err != nil || limited.Unlimited() {
		t.Fatalf("NewLimits = (%+v, %v)", limited, err)
	}
}

func intPointer(value int) *int { return &value }

func int64Pointer(value int64) *int64 { return &value }

func floatPointer(value float64) *float64 { return &value }

// TestOutcomeTerminalState pins the outcome → terminal-state mapping: completion
// and cancellation get their own states; every failure flavor folds into Failed.
func TestOutcomeTerminalState(t *testing.T) {
	want := map[Outcome]State{
		OutcomeCompleted: Completed,
		OutcomeCanceled:  Canceled,
		OutcomeTimedOut:  Failed,
		OutcomeFailed:    Failed,
		OutcomeMaxBudget: Failed,
		OutcomeMaxSteps:  Failed,
		OutcomeLost:      Failed,
	}
	for _, o := range allOutcomes {
		if got := o.terminalState(); got != want[o] {
			t.Errorf("%s.terminalState() = %s, want %s", o, got, want[o])
		}
	}
}

func TestOutcomeStringRoundTrip(t *testing.T) {
	for _, outcome := range allOutcomes {
		parsed, ok := ParseOutcome(outcome.String())
		if !ok || parsed != outcome {
			t.Errorf("ParseOutcome(%q) = (%s, %v), want (%s, true)", outcome, parsed, ok, outcome)
		}
	}
	if _, ok := ParseOutcome("error"); ok {
		t.Fatal("ParseOutcome accepted removed error terminology")
	}
}

// TestNoTransitionFromTerminal: once terminal, no operation advances the run.
func TestNoTransitionFromTerminal(t *testing.T) {
	for _, s := range []State{Completed, Failed, Canceled} {
		if _, ok := s.Suspend(); ok {
			t.Errorf("%s.Suspend() unexpectedly succeeded", s)
		}
		if _, ok := s.Resume(); ok {
			t.Errorf("%s.Resume() unexpectedly succeeded", s)
		}
		for _, o := range allOutcomes {
			if _, ok := s.Terminate(o); ok {
				t.Errorf("%s.Terminate(%s) unexpectedly succeeded", s, o)
			}
		}
	}
}

// TestStringsAreDistinct guards the String() maps against a copy-paste collision
// (two states or two outcomes sharing a label) and the "unknown" fallthrough.
func TestStringsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range allStates {
		if s.String() == "unknown" {
			t.Errorf("state %q stringifies as unknown", s)
		}
		if seen[s.String()] {
			t.Errorf("duplicate state label %q", s.String())
		}
		seen[s.String()] = true
	}
	seen = map[string]bool{}
	for _, o := range allOutcomes {
		if o.String() == "unknown" {
			t.Errorf("outcome %q stringifies as unknown", o)
		}
		if seen[o.String()] {
			t.Errorf("duplicate outcome label %q", o.String())
		}
		seen[o.String()] = true
	}
}

// TestDurability pins the commit-before-publish predicate.
