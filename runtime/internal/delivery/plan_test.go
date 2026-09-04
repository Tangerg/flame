package delivery

import (
	"context"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func saveTestPlan(ctx context.Context, store *sqlite.PlanStore, sessionID string, steps []plan.Step) error {
	current, err := store.State(ctx, sessionID)
	if err != nil {
		return err
	}
	updatedAt := time.Unix(1, 0).UTC()
	if committed, ok := current.State(); ok {
		updatedAt = committed.UpdatedAt().Add(time.Nanosecond)
	}
	replacement, err := current.Replace(steps, updatedAt)
	if err != nil {
		return err
	}
	change, err := plan.NewReplacement(current.Version(), replacement)
	if err != nil {
		return err
	}
	return store.Save(ctx, sessionID, change)
}
