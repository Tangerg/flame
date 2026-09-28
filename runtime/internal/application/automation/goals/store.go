package goals

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
)

// Store returns restored Domain values. Get addresses the exact Session; List
// contains each Session once. Decoding and row integrity belong to the store.
type Store interface {
	Get(ctx context.Context, sessionID string) (goal.Current, error)
	// Save executes one domain-decided exact durable replacement. Persistence
	// never assigns or rewrites Goal identity.
	// A lost compare-and-swap returns applied=false.
	Save(ctx context.Context, replacement goal.Replacement) (applied bool, err error)
	ClearIf(ctx context.Context, sessionID string, expected goal.Version) (applied bool, err error)
	List(ctx context.Context) ([]goal.Goal, error)
}

func loadGoal(ctx context.Context, store Store, sessionID string) (goal.Goal, bool, error) {
	current, err := store.Get(ctx, sessionID)
	if err != nil {
		return goal.Goal{}, false, err
	}
	value, exists := current.Goal()
	return value, exists, nil
}
