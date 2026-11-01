package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
)

func decide(request goalworkflow.Request) goalworkflow.Decision {
	invalid := goalworkflow.Decision{Refusal: goalworkflow.Invalid}
	objective := strings.TrimSpace(request.Objective)
	if request.Signal == goalworkflow.Start {
		if objective == "" {
			return invalid
		}
		return goalworkflow.Decision{Next: &goalworkflow.Value{Objective: objective, Status: goalworkflow.GoalActive}}
	}
	if request.Current == nil {
		return invalid
	}
	current := *request.Current
	next := goalworkflow.Value{Objective: current.Objective}
	switch request.Signal {
	case goalworkflow.NextRun:
		if current.Status != goalworkflow.GoalActive || request.Ordinal == 0 {
			return invalid
		}
		return goalworkflow.Decision{Instructions: runInstructions(current.Objective, request.Ordinal > 1)}
	case goalworkflow.Resume:
		if current.Status != goalworkflow.GoalPaused && current.Status != goalworkflow.GoalBlocked {
			return goalworkflow.Decision{Refusal: goalworkflow.NotResumable}
		}
		next.Status = goalworkflow.GoalActive
	case goalworkflow.Stop:
		next.Status, next.Reason = goalworkflow.GoalPaused, goalworkflow.GoalReasonStoppedByUser
	case goalworkflow.Revise, goalworkflow.ReviseAndResume:
		if current.Status == goalworkflow.GoalCompleting || objective == "" {
			return invalid
		}
		if request.Signal == goalworkflow.ReviseAndResume {
			if current.Status != goalworkflow.GoalPaused {
				return invalid
			}
			next.Status = goalworkflow.GoalActive
		} else {
			next = current
		}
		next.Objective = objective
	default:
		return activeDecision(current, request)
	}
	return goalworkflow.Decision{Next: &next}
}

func activeDecision(current goalworkflow.Value, request goalworkflow.Request) goalworkflow.Decision {
	next := goalworkflow.Value{Objective: current.Objective}
	if request.Signal == goalworkflow.TerminalRun {
		if current.Status != goalworkflow.GoalActive || request.Outcome == runoutcome.Completed {
			return goalworkflow.Decision{}
		}
		next.Status, next.Reason, next.Outcome = goalworkflow.GoalPaused, goalworkflow.GoalReasonRunNotCompleted, request.Outcome
		return goalworkflow.Decision{Next: &next}
	}
	if current.Status != goalworkflow.GoalActive {
		return goalworkflow.Decision{Refusal: goalworkflow.Invalid}
	}
	switch request.Signal {
	case goalworkflow.Pause:
		next.Status, next.Reason = goalworkflow.GoalPaused, request.Reason
	case goalworkflow.Complete:
		next.Status = goalworkflow.GoalCompleting
	case goalworkflow.Block:
		if strings.TrimSpace(request.Explanation) == "" {
			return goalworkflow.Decision{Refusal: goalworkflow.BlockedReasonRequired}
		}
		next.Status, next.Reason, next.Explanation = goalworkflow.GoalBlocked, goalworkflow.GoalReasonBlockedByModel, strings.TrimSpace(request.Explanation)
	default:
		return goalworkflow.Decision{Refusal: goalworkflow.Invalid}
	}
	return goalworkflow.Decision{Next: &next}
}
