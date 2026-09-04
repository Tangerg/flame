package goals

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

// Reader exposes current Goal state without persistence or mutation operations.
type Reader struct {
	goals Store
}

// NewReader returns the read boundary over store. A nil store leaves Goal mode
// unavailable, so composition should omit the reader.
func NewReader(store Store) *Reader {
	if store == nil {
		return nil
	}
	return &Reader{goals: store}
}

// Current returns the session's current Goal.
func (r *Reader) Current(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	if r == nil || r.goals == nil {
		return goal.Goal{}, false, nil
	}
	return loadGoal(ctx, r.goals, sessionID)
}

// Active reports whether sessionID currently has an actively driven Goal.
func (r *Reader) Active(ctx context.Context, sessionID string) (bool, error) {
	if r == nil || r.goals == nil {
		return false, nil
	}
	current, exists, err := loadGoal(ctx, r.goals, sessionID)
	if err != nil {
		return false, err
	}
	return exists && current.Status() == goal.StatusActive, nil
}
