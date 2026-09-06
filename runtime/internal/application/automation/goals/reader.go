package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

// Reader exposes current Goal state without persistence or mutation operations.
type Reader struct {
	goals Store
}

// NewReader constructs the read boundary over the required Goal store.
func NewReader(store Store) (*Reader, error) {
	if missingDependency(store) {
		return nil, errors.New("goals: reader store is required")
	}
	return &Reader{goals: store}, nil
}

// Current returns the session's current Goal.
func (r *Reader) Current(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	return loadGoal(ctx, r.goals, sessionID)
}

// Active reports whether sessionID currently has an actively driven Goal.
func (r *Reader) Active(ctx context.Context, sessionID string) (bool, error) {
	current, exists, err := loadGoal(ctx, r.goals, sessionID)
	if err != nil {
		return false, err
	}
	return exists && current.Status() == goal.StatusActive, nil
}
