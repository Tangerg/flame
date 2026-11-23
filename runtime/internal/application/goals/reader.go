package goals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/samber/lo"
)

type Reader struct {
	goals Store
}

func NewReader(store Store) (*Reader, error) {
	if lo.IsNil(store) {
		return nil, errors.New("goals: reader store is required")
	}
	return &Reader{goals: store}, nil
}

func (r *Reader) Current(ctx context.Context, sessionID identity.SessionID) (goal.Goal, bool, error) {
	return loadGoal(ctx, r.goals, sessionID)
}
