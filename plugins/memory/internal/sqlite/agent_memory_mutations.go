package sqlite

import (
	"context"
	"time"

	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/flame/plugins/memory/internal/management"
	framesqlite "github.com/Tangerg/frame/sqlite"
)

// Standalone callers need the same decision and write to share one lock. Receipt
// commands instead prepare under their own read lease and apply the decided plan.
func (a *AgentMemoryStore) mutate(ctx context.Context, prepare func(context.Context) (management.Mutation, error)) (management.Mutation, error) {
	var mutation management.Mutation
	err := framesqlite.RunInTx(ctx, a.db, func(ctx context.Context) error {
		var err error
		mutation, err = prepare(ctx)
		if err != nil {
			return err
		}
		return mutation.Apply(ctx)
	})
	if err != nil {
		return management.Mutation{}, err
	}
	return mutation, nil
}

func (a *AgentMemoryStore) Add(ctx context.Context, scope memory.Scope, project, content string, now time.Time) (memory.Item, bool, error) {
	mutation, err := a.mutate(ctx, func(ctx context.Context) (management.Mutation, error) {
		return a.PrepareAdd(ctx, scope, project, content, now)
	})
	return mutation.Item(), mutation.Changed(), err
}

func (a *AgentMemoryStore) Update(ctx context.Context, id memory.ItemID, expectedRevision uint64, content *string, pinned *bool, now time.Time) (memory.Item, error) {
	mutation, err := a.mutate(ctx, func(ctx context.Context) (management.Mutation, error) {
		return a.PrepareUpdate(ctx, id, expectedRevision, content, pinned, now)
	})
	return mutation.Item(), err
}

func (a *AgentMemoryStore) Review(ctx context.Context, id memory.ItemID, expectedRevision uint64, decision memory.ReviewDecision, now time.Time) error {
	_, err := a.mutate(ctx, func(ctx context.Context) (management.Mutation, error) {
		return a.PrepareReview(ctx, id, expectedRevision, decision, now)
	})
	return err
}

func (a *AgentMemoryStore) Delete(ctx context.Context, id memory.ItemID, expectedRevision uint64) error {
	_, err := a.mutate(ctx, func(ctx context.Context) (management.Mutation, error) {
		return a.PrepareDelete(ctx, id, expectedRevision)
	})
	return err
}
