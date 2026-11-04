package goaltest

import (
	"context"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
)

// These canned replies let coordinator tests exercise competing storage and Run
// boundaries without starting a backend. Publisher tests exercise its rules.
type Workflow struct{}

func (Workflow) RequireGoalAuthor(context.Context, extension.ContributionSource) error { return nil }

func (Workflow) DecideGoal(_ context.Context, source extension.ContributionSource, r goalworkflow.Request) (goalworkflow.AuthoredDecision, error) {
	if source == (extension.ContributionSource{}) {
		source = extension.ContributionSource{InstallationID: "eeb329cd-c7ce-40c9-bd90-6821fef06d30", Contribution: "test", Release: framevalue.Hash([]byte("goal")).String()}
	}
	value, err := cannedGoalDecision(r)
	return goalworkflow.AuthoredDecision{Source: source, Decision: value}, err
}
func cannedGoalDecision(r goalworkflow.Request) (goalworkflow.Decision, error) {
	var next goalworkflow.Value
	if r.Current != nil {
		next = *r.Current
	}
	wake := func() {
		next.Status = goalworkflow.GoalActive
		next.Reason = ""
		next.Outcome = ""
		next.Explanation = ""
	}
	switch r.Signal {
	case goalworkflow.NextRun:
		return goalworkflow.Decision{Instructions: next.Objective}, nil
	case goalworkflow.Start:
		wake()
		next.Objective = strings.TrimSpace(r.Objective)
	case goalworkflow.Resume:
		if next.Status != goalworkflow.GoalPaused && next.Status != goalworkflow.GoalBlocked {
			return goalworkflow.Decision{Refusal: goalworkflow.NotResumable}, nil
		}
		wake()
	case goalworkflow.Revise, goalworkflow.ReviseAndResume:
		if r.Signal == goalworkflow.ReviseAndResume {
			wake()
		}
		next.Objective = strings.TrimSpace(r.Objective)
	case goalworkflow.Stop:
		next.Status, next.Reason, next.Outcome, next.Explanation = goalworkflow.GoalPaused, goalworkflow.GoalReasonStoppedByUser, "", ""
	case goalworkflow.Pause:
		next.Status, next.Reason = goalworkflow.GoalPaused, r.Reason
	case goalworkflow.Complete:
		next.Status = goalworkflow.GoalCompleting
	case goalworkflow.Block:
		if strings.TrimSpace(r.Explanation) == "" {
			return goalworkflow.Decision{Refusal: goalworkflow.BlockedReasonRequired}, nil
		}
		next.Status, next.Reason, next.Explanation = goalworkflow.GoalBlocked, goalworkflow.GoalReasonBlockedByModel, r.Explanation
	case goalworkflow.TerminalRun:
		if next.Status != goalworkflow.GoalActive || r.Outcome == runoutcome.Completed {
			return goalworkflow.Decision{}, nil
		}
		next.Status, next.Reason, next.Outcome = goalworkflow.GoalPaused, goalworkflow.GoalReasonRunNotCompleted, r.Outcome
	}
	return goalworkflow.Decision{Next: &next}, nil
}
