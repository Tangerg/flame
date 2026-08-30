package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/goal"
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

func bootstrapUnwrittenGoalVersion(t *testing.T, sessionID string) goal.Version {
	t.Helper()
	current, err := goal.Unwritten(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return current.Version()
}
