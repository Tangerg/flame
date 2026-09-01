package agent

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestGoalRejectsMissingOrRegressingDurableTime(t *testing.T) {
	createdAt := time.Unix(10, 0).UTC()
	valid := GoalSnapshot{
		SessionID: "ses_1", Objective: "finish the task", Status: GoalActive,
		Budget: UnlimitedGoalBudget(), CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	if _, err := RestoreGoal(valid); err != nil {
		t.Fatalf("valid Goal: %v", err)
	}

	missing := valid
	missing.CreatedAt = time.Time{}
	if _, err := RestoreGoal(missing); err == nil {
		t.Fatal("Goal without a creation time was accepted")
	}

	regressing := valid
	regressing.UpdatedAt = createdAt.Add(-time.Nanosecond)
	if _, err := RestoreGoal(regressing); err == nil {
		t.Fatal("Goal whose update precedes creation was accepted")
	}
}

func TestGoalLifecycleValuesRejectAmbiguousState(t *testing.T) {
	activeSnapshot := validGoalSnapshot()
	active := restoreGoal(t, activeSnapshot)
	activeWithReason := activeSnapshot
	activeWithReason.ReasonCode = GoalStoppedByUser
	if _, err := RestoreGoal(activeWithReason); err == nil {
		t.Fatal("active goal with a stop reason was accepted")
	}
	paused := activeSnapshot
	paused.Status = GoalPaused
	if _, err := RestoreGoal(paused); err == nil {
		t.Fatal("paused goal without a reason was accepted")
	}
	completingSnapshot := activeSnapshot
	completingSnapshot.Status = GoalCompleting
	completing := restoreGoal(t, completingSnapshot)
	completingWithReason := completingSnapshot
	completingWithReason.ReasonCode = GoalRunNotCompleted
	completingWithReason.ReasonDetail = "unfinished"
	if _, err := RestoreGoal(completingWithReason); err == nil {
		t.Fatal("completing goal with a stop reason was accepted")
	}
	if completing.Status().AllowsLifecycleCommands() || !active.Status().AllowsLifecycleCommands() {
		t.Fatal("goal lifecycle command policy does not distinguish settlement")
	}
	if err := (StartGoal{SessionID: "ses_1", Objective: "finish"}).Validate(); err == nil {
		t.Fatal("implicit zero goal budget was accepted")
	}
	if _, err := NewGoalBudget(GoalBudgetLimits{MaxCostUSD: floatLimit(math.NaN())}); err == nil {
		t.Fatal("NaN goal budget limit was accepted")
	}
	if _, err := NewGoalUsage(0, math.Inf(1), 0); err == nil {
		t.Fatal("infinite goal usage was accepted")
	}
	if err := (StartGoal{SessionID: "ses_1", Objective: "finish", Provider: " anthropic", Model: "deep"}).Validate(); err == nil {
		t.Fatal("non-canonical model selection was accepted")
	}
	if err := (StartGoal{SessionID: "ses_1", Objective: "finish", ReasoningEffort: "high", Budget: UnlimitedGoalBudget()}).Validate(); err == nil {
		t.Fatal("reasoning effort without a model was accepted")
	}
}

func TestGoalReasonAndBudgetMustMatchLifecycle(t *testing.T) {
	limited := limitedBudget(t, GoalBudgetLimits{MaxRuns: intLimit(1)})
	oneRun, err := NewGoalUsage(1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*GoalSnapshot)
	}{
		{name: "pause with budget reason", mutate: func(snapshot *GoalSnapshot) {
			snapshot.Status = GoalPaused
			snapshot.ReasonCode = GoalRunBudgetReached
		}},
		{name: "block with user stop", mutate: func(snapshot *GoalSnapshot) {
			snapshot.Status = GoalBlocked
			snapshot.ReasonCode = GoalStoppedByUser
		}},
		{name: "model block without explanation", mutate: func(snapshot *GoalSnapshot) {
			snapshot.Status = GoalBlocked
			snapshot.ReasonCode = GoalBlockedByModel
		}},
		{name: "unfinished run without outcome", mutate: func(snapshot *GoalSnapshot) {
			snapshot.Status = GoalPaused
			snapshot.ReasonCode = GoalRunNotCompleted
		}},
		{name: "active exhausted budget", mutate: func(snapshot *GoalSnapshot) {
			snapshot.Budget = limited
			snapshot.Used = oneRun
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validGoalSnapshot()
			test.mutate(&snapshot)
			if _, err := RestoreGoal(snapshot); err == nil {
				t.Fatal("contradictory Goal was accepted")
			}
		})
	}
}

func TestGoalAcceptsPricingUnavailableBlock(t *testing.T) {
	snapshot := validGoalSnapshot()
	snapshot.Status = GoalBlocked
	snapshot.ReasonCode = GoalPricingUnavailable
	if _, err := RestoreGoal(snapshot); err != nil {
		t.Fatalf("RestoreGoal: %v", err)
	}
}

func TestGoalStartResultMustFulfillTheCommand(t *testing.T) {
	budget := limitedBudget(t, GoalBudgetLimits{MaxRuns: intLimit(3), MaxCostUSD: floatLimit(1.5), MaxSteps: intLimit(20)})
	start := StartGoal{
		SessionID: "ses_1", Objective: "finish", Provider: "anthropic", Model: "deep", ReasoningEffort: "high",
		Budget: budget,
	}
	validSnapshot := GoalSnapshot{
		SessionID: start.SessionID, Objective: start.Objective, Status: GoalActive,
		Provider: start.Provider, Model: start.Model, ReasoningEffort: start.ReasoningEffort, Budget: start.Budget,
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(),
	}
	valid := restoreGoal(t, validSnapshot)
	if err := start.ValidateResult(valid); err != nil {
		t.Fatalf("valid start result: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*GoalSnapshot)
		want   string
	}{
		{name: "session", mutate: func(result *GoalSnapshot) { result.SessionID = "ses_other" }, want: "session"},
		{name: "objective", mutate: func(result *GoalSnapshot) { result.Objective = "ignored" }, want: "objective"},
		{name: "status", mutate: func(result *GoalSnapshot) {
			result.Status = GoalPaused
			result.ReasonCode = GoalStoppedByUser
		}, want: "status"},
		{name: "model", mutate: func(result *GoalSnapshot) { result.Model = "shallow" }, want: "model"},
		{name: "reasoning effort", mutate: func(result *GoalSnapshot) { result.ReasoningEffort = "medium" }, want: "model selection"},
		{name: "budget", mutate: func(result *GoalSnapshot) {
			result.Budget = limitedBudget(t, GoalBudgetLimits{MaxRuns: intLimit(4)})
		}, want: "budget"},
		{name: "usage", mutate: func(result *GoalSnapshot) { result.Used, _ = NewGoalUsage(1, 0, 0) }, want: "non-zero usage"},
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

func validGoalSnapshot() GoalSnapshot {
	at := time.Unix(1, 0).UTC()
	return GoalSnapshot{
		SessionID: "ses_1", Objective: "finish the task", Status: GoalActive,
		Budget: UnlimitedGoalBudget(), CreatedAt: at, UpdatedAt: at,
	}
}

func restoreGoal(t testing.TB, snapshot GoalSnapshot) Goal {
	t.Helper()
	value, err := RestoreGoal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestBudgetDistinguishesUnlimitedFromPositiveLimits(t *testing.T) {
	if err := (GoalBudget{}).Validate(); err == nil {
		t.Fatal("zero GoalBudget was accepted")
	}
	if err := (GoalBudget{initialized: true, maxRuns: -1}).Validate(); err == nil {
		t.Fatal("corrupt negative GoalBudget was accepted")
	}
	if budget := UnlimitedGoalBudget(); !budget.Unlimited() || budget.Validate() != nil {
		t.Fatalf("UnlimitedGoalBudget = %+v", budget)
	}
	for _, test := range []struct {
		name   string
		limits GoalBudgetLimits
	}{
		{name: "empty"},
		{name: "zero runs", limits: GoalBudgetLimits{MaxRuns: intLimit(0)}},
		{name: "zero cost", limits: GoalBudgetLimits{MaxCostUSD: floatLimit(0)}},
		{name: "zero steps", limits: GoalBudgetLimits{MaxSteps: intLimit(0)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewGoalBudget(test.limits); err == nil {
				t.Fatal("NewGoalBudget accepted a missing or zero limit")
			}
		})
	}
}

func limitedBudget(t *testing.T, limits GoalBudgetLimits) GoalBudget {
	t.Helper()
	budget, err := NewGoalBudget(limits)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}

func intLimit(value int) *int { return &value }

func floatLimit(value float64) *float64 { return &value }
