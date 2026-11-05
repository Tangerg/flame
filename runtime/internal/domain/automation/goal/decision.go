package goal

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func (state Snapshot) WorkflowValue() goalworkflow.Value {
	status := goalworkflow.GoalStatus(state.Status)
	if state.Status == StatusComplete {
		status = goalworkflow.GoalCompleting
	}
	return goalworkflow.Value{Objective: state.Objective, Status: status, Reason: goalworkflow.GoalReasonCode(state.ReasonCode), Outcome: runoutcome.Kind(state.ReasonRunOutcome), Explanation: state.ReasonExplanation}
}

// Publish advances only the storage envelope. Workflow actions and their
// admissible successors belong to the captured author.
func (state Snapshot) Publish(value goalworkflow.Value, author extension.ContributionSource, incarnation string, now time.Time) (Goal, error) {
	if state.Author != (extension.ContributionSource{}) && state.Author != author {
		return Goal{}, errors.New("goal: author differs from captured incarnation")
	}
	if err := author.Validate(); err != nil {
		return Goal{}, err
	}
	state.Author = author
	if now.Before(state.UpdatedAt) {
		return Goal{}, errors.New("goals: decision predates captured state")
	}
	state.UpdatedAt = now

	state.Objective, state.Status, state.ReasonCode = value.Objective, Status(value.Status), ReasonCode(value.Reason)
	if value.Status == goalworkflow.GoalCompleting {
		state.Status = StatusComplete
	}
	state.ReasonRunOutcome, state.ReasonExplanation = run.Outcome(value.Outcome), value.Explanation
	if state.Revision == 0 || incarnation != state.IncarnationID {
		state.IncarnationID, state.Revision, state.Used = incarnation, exactint.First().Value(), Usage{}
	} else {
		next, err := exactint.Restore(state.Revision)
		if err != nil {
			return Goal{}, err
		}
		next, err = next.Next()
		if err != nil {
			return Goal{}, err
		}
		state.Revision = next.Value()
	}
	return Restore(state)
}
