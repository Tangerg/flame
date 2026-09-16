package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

// Reader exposes current Goal state without persistence or mutation operations.
type Reader struct {
	goals Store
}

// NewReader constructs the read boundary over the required Goal store.
func NewReader(store Store) (*Reader, error) {
	if dependency.Missing(store) {
		return nil, errors.New("goals: reader store is required")
	}
	return &Reader{goals: store}, nil
}

// Current returns the session's current Goal.
func (r *Reader) Current(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	return loadGoal(ctx, r.goals, sessionID)
}
