package main

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
)

func TestRunInstructionsUseOutcomeReportingContract(t *testing.T) {
	instructions := runInstructions("finish migration", true)
	for _, want := range []string{
		"Continue toward the goal: finish migration",
		`report_goal_outcome(outcome="completed")`,
		`report_goal_outcome(outcome="blocked", reason="...")`,
	} {
		if !strings.Contains(instructions, want) {
			t.Errorf("RunInstructions() = %q, missing %q", instructions, want)
		}
	}
	for _, legacy := range []string{"update_goal", `status="complete"`} {
		if strings.Contains(instructions, legacy) {
			t.Errorf("RunInstructions() = %q, contains legacy contract %q", instructions, legacy)
		}
	}
}

func TestGoalAuthorDecidesTransitions(t *testing.T) {
	active := goalworkflow.Value{Objective: "original", Status: goalworkflow.GoalActive}
	cases := []struct {
		name    string
		input   goalworkflow.Request
		want    goalworkflow.GoalStatus
		reason  goalworkflow.GoalReasonCode
		refusal goalworkflow.Refusal
	}{
		{"start", goalworkflow.Request{Signal: goalworkflow.Start, Objective: " first "}, goalworkflow.GoalActive, "", ""},
		{"pause", goalworkflow.Request{Signal: goalworkflow.Pause, Current: &active, Reason: goalworkflow.GoalReasonAwaitingInput}, goalworkflow.GoalPaused, goalworkflow.GoalReasonAwaitingInput, ""},
		{"stop", goalworkflow.Request{Signal: goalworkflow.Stop, Current: &active}, goalworkflow.GoalPaused, goalworkflow.GoalReasonStoppedByUser, ""},
		{"failure", goalworkflow.Request{Signal: goalworkflow.TerminalRun, Current: &active, Outcome: runoutcome.Failed}, goalworkflow.GoalPaused, goalworkflow.GoalReasonRunNotCompleted, ""},
		{"complete", goalworkflow.Request{Signal: goalworkflow.Complete, Current: &active}, goalworkflow.GoalCompleting, "", ""},
		{"block", goalworkflow.Request{Signal: goalworkflow.Block, Current: &active, Explanation: "explain"}, goalworkflow.GoalBlocked, goalworkflow.GoalReasonBlockedByModel, ""},
		{"block missing detail", goalworkflow.Request{Signal: goalworkflow.Block, Current: &active}, "", "", goalworkflow.BlockedReasonRequired},
		{"unresumable", goalworkflow.Request{Signal: goalworkflow.Resume, Current: &active}, "", "", goalworkflow.NotResumable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := decide(c.input)
			if result.Refusal != c.refusal {
				t.Fatalf("refusal=%s", result.Refusal)
			}
			if c.refusal != "" {
				if result.Next != nil {
					t.Fatal("refused decision carried a successor")
				}
				return
			}
			if result.Next == nil || result.Next.Status != c.want || result.Next.Reason != c.reason {
				t.Fatalf("next=%+v", result.Next)
			}
		})
	}
	finished := decide(goalworkflow.Request{Signal: goalworkflow.TerminalRun, Current: &active, Outcome: runoutcome.Completed})
	if finished.Next != nil || finished.Refusal != "" {
		t.Fatal("a successful Run alone completed an objective")
	}
}
