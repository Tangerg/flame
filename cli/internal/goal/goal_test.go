package goal

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestGoalRejectsMissingOrRegressingDurableTime(t *testing.T) {
	createdAt := time.Unix(10, 0).UTC()
	valid := Snapshot{
		SessionID: "ses_1", Objective: "finish the task", Status: Active,
		Budget: UnlimitedBudget(), CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	if _, err := Restore(valid); err != nil {
		t.Fatalf("valid Goal: %v", err)
	}

	missing := valid
	missing.CreatedAt = time.Time{}
	if _, err := Restore(missing); err == nil {
		t.Fatal("Goal without a creation time was accepted")
	}

	regressing := valid
	regressing.UpdatedAt = createdAt.Add(-time.Nanosecond)
	if _, err := Restore(regressing); err == nil {
		t.Fatal("Goal whose update precedes creation was accepted")
	}
}

func TestGoalLifecycleValuesRejectAmbiguousState(t *testing.T) {
	activeSnapshot := validGoalSnapshot()
	active := restoreGoal(t, activeSnapshot)
	activeWithReason := activeSnapshot
	activeWithReason.ReasonCode = StoppedByUser
	if _, err := Restore(activeWithReason); err == nil {
		t.Fatal("active goal with a stop reason was accepted")
	}
	paused := activeSnapshot
	paused.Status = Paused
	if _, err := Restore(paused); err == nil {
		t.Fatal("paused goal without a reason was accepted")
	}
	completingSnapshot := activeSnapshot
	completingSnapshot.Status = Completing
	completing := restoreGoal(t, completingSnapshot)
	completingWithReason := completingSnapshot
	completingWithReason.ReasonCode = RunNotCompleted
	completingWithReason.ReasonDetail = "unfinished"
	if _, err := Restore(completingWithReason); err == nil {
		t.Fatal("completing goal with a stop reason was accepted")
	}
	if completing.Status().AllowsLifecycleCommands() || !active.Status().AllowsLifecycleCommands() {
		t.Fatal("goal lifecycle command policy does not distinguish settlement")
	}
	if err := (Start{SessionID: "ses_1", Objective: "finish"}).Validate(); err == nil {
		t.Fatal("implicit zero goal budget was accepted")
	}
	if _, err := NewBudget(BudgetLimits{MaxCostUSD: floatLimit(math.NaN())}); err == nil {
		t.Fatal("NaN goal budget limit was accepted")
	}
	if _, err := NewUsage(0, math.Inf(1), 0); err == nil {
		t.Fatal("infinite goal usage was accepted")
	}
	if err := (Start{SessionID: "ses_1", Objective: "finish", Provider: " anthropic", Model: "deep"}).Validate(); err == nil {
		t.Fatal("non-canonical model selection was accepted")
	}
}

func TestGoalReasonAndBudgetMustMatchLifecycle(t *testing.T) {
	limited := limitedBudget(t, BudgetLimits{MaxRuns: intLimit(1)})
	oneRun, err := NewUsage(1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{name: "pause with budget reason", mutate: func(snapshot *Snapshot) {
			snapshot.Status = Paused
			snapshot.ReasonCode = RunBudgetReached
		}},
		{name: "block with user stop", mutate: func(snapshot *Snapshot) {
			snapshot.Status = Blocked
			snapshot.ReasonCode = StoppedByUser
		}},
		{name: "model block without explanation", mutate: func(snapshot *Snapshot) {
			snapshot.Status = Blocked
			snapshot.ReasonCode = BlockedByModel
		}},
		{name: "unfinished run without outcome", mutate: func(snapshot *Snapshot) {
			snapshot.Status = Paused
			snapshot.ReasonCode = RunNotCompleted
		}},
		{name: "active exhausted budget", mutate: func(snapshot *Snapshot) {
			snapshot.Budget = limited
			snapshot.Used = oneRun
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validGoalSnapshot()
			test.mutate(&snapshot)
			if _, err := Restore(snapshot); err == nil {
				t.Fatal("contradictory Goal was accepted")
			}
		})
	}
}

func TestGoalStartResultMustFulfillTheCommand(t *testing.T) {
	budget := limitedBudget(t, BudgetLimits{MaxRuns: intLimit(3), MaxCostUSD: floatLimit(1.5), MaxSteps: intLimit(20)})
	start := Start{
		SessionID: "ses_1", Objective: "finish", Provider: "anthropic", Model: "deep",
		Budget: budget,
	}
	validSnapshot := Snapshot{
		SessionID: start.SessionID, Objective: start.Objective, Status: Active,
		Provider: start.Provider, Model: start.Model, Budget: start.Budget,
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(),
	}
	valid := restoreGoal(t, validSnapshot)
	if err := start.ValidateResult(valid); err != nil {
		t.Fatalf("valid start result: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{name: "session", mutate: func(result *Snapshot) { result.SessionID = "ses_other" }, want: "session"},
		{name: "objective", mutate: func(result *Snapshot) { result.Objective = "ignored" }, want: "objective"},
		{name: "status", mutate: func(result *Snapshot) {
			result.Status = Paused
			result.ReasonCode = StoppedByUser
		}, want: "status"},
		{name: "model", mutate: func(result *Snapshot) { result.Model = "shallow" }, want: "model"},
		{name: "budget", mutate: func(result *Snapshot) {
			result.Budget = limitedBudget(t, BudgetLimits{MaxRuns: intLimit(4)})
		}, want: "budget"},
		{name: "usage", mutate: func(result *Snapshot) { result.Used, _ = NewUsage(1, 0, 0) }, want: "non-zero usage"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultSnapshot := valid.Snapshot()
			test.mutate(&resultSnapshot)
			result := restoreGoal(t, resultSnapshot)
			err := start.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}
}

func validGoalSnapshot() Snapshot {
	at := time.Unix(1, 0).UTC()
	return Snapshot{
		SessionID: "ses_1", Objective: "finish the task", Status: Active,
		Budget: UnlimitedBudget(), CreatedAt: at, UpdatedAt: at,
	}
}

func restoreGoal(t testing.TB, snapshot Snapshot) Goal {
	t.Helper()
	value, err := Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestBudgetDistinguishesUnlimitedFromPositiveLimits(t *testing.T) {
	if err := (Budget{}).Validate(); err == nil {
		t.Fatal("zero Budget was accepted")
	}
	if err := (Budget{initialized: true, maxRuns: -1}).Validate(); err == nil {
		t.Fatal("corrupt negative Budget was accepted")
	}
	if budget := UnlimitedBudget(); !budget.Unlimited() || budget.Validate() != nil {
		t.Fatalf("UnlimitedBudget = %+v", budget)
	}
	for _, test := range []struct {
		name   string
		limits BudgetLimits
	}{
		{name: "empty"},
		{name: "zero runs", limits: BudgetLimits{MaxRuns: intLimit(0)}},
		{name: "zero cost", limits: BudgetLimits{MaxCostUSD: floatLimit(0)}},
		{name: "zero steps", limits: BudgetLimits{MaxSteps: intLimit(0)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewBudget(test.limits); err == nil {
				t.Fatal("NewBudget accepted a missing or zero limit")
			}
		})
	}
}

func limitedBudget(t *testing.T, limits BudgetLimits) Budget {
	t.Helper()
	budget, err := NewBudget(limits)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func intLimit(value int) *int { return &value }

func floatLimit(value float64) *float64 { return &value }
