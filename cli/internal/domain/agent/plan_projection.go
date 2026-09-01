package agent

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/protocol"
)

func clonePlan(plan *protocol.Plan) *protocol.Plan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	if plan.State != nil {
		state := *plan.State
		state.Steps = slices.Clone(plan.State.Steps)
		cloned.State = &state
	}
	return &cloned
}

func equalPlans(left, right *protocol.Plan) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.SessionID != right.SessionID || (left.State == nil) != (right.State == nil) {
		return false
	}
	if left.State == nil {
		return true
	}
	return left.State.Revision == right.State.Revision && slices.Equal(left.State.Steps, right.State.Steps)
}

func committedPlanState(plan *protocol.Plan) (*protocol.PlanState, error) {
	if plan == nil || plan.State == nil {
		return nil, errors.New("plan has no committed state")
	}
	return plan.State, nil
}
