package goaltest

import (
	"context"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"time"
)

func New(sessionID, objective string, selection modelchoice.Selection, capabilities run.Capabilities, incarnation string, now time.Time) (goal.Goal, error) {
	state := goal.Snapshot{SessionID: sessionID, CreatedAt: now, ModelSelection: selection, Capabilities: capabilities.Normalized()}
	return Decide(state, goalworkflow.Request{Signal: goalworkflow.Start, Objective: objective}, incarnation, now)
}
func Decide(state goal.Snapshot, input goalworkflow.Request, incarnation string, now time.Time) (goal.Goal, error) {
	if state.Revision != 0 {
		current := state.WorkflowValue()
		input.Current = &current
	}
	decision, err := (Workflow{}).DecideGoal(context.Background(), state.Author, input)
	if err != nil {
		return goal.Goal{}, err
	}
	if decision.Next == nil {
		return goal.Restore(state)
	}
	return state.Publish(*decision.Next, decision.Source, incarnation, now)
}
func Paused(g goal.Goal, why goal.ReasonCode, now time.Time) (goal.Goal, error) {
	return Decide(g.Snapshot(), goalworkflow.Request{Signal: goalworkflow.Pause, Reason: goalworkflow.GoalReasonCode(why)}, g.IncarnationID(), now)
}
func Completed(g goal.Goal, now time.Time) (goal.Goal, error) {
	return Decide(g.Snapshot(), goalworkflow.Request{Signal: goalworkflow.Complete}, g.IncarnationID(), now)
}
func Resumed(g goal.Goal, now time.Time) (goal.Goal, error) {
	return Decide(g.Snapshot(), goalworkflow.Request{Signal: goalworkflow.Resume}, g.IncarnationID(), now)
}
