package bootstrap

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

type bootstrapGoalReader interface {
	Get(context.Context, string) (goal.Current, error)
}

func readBootstrapGoal(ctx context.Context, store bootstrapGoalReader, sessionID string) (goal.Goal, bool, error) {
	current, err := store.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, false, err
	}
	value, exists := current.Goal()
	return value, exists, nil
}
