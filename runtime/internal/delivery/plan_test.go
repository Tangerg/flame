package delivery

import (
	"context"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

func saveTestPlan(ctx context.Context, store *persistence.PlanStore, sessionID string, steps []plan.Step) error {
	current, err := store.State(ctx, sessionID)
	if err != nil {
		return err
	}
	doc, err := (testsupport.PlanAuthor{}).AuthorPlan(ctx, extension.ContributionSource{}, plan.ProposalOf(steps))
	if err != nil {
		return err
	}
	updatedAt := time.Unix(1, 0).UTC()
	if committed, ok := current.State(); ok {
		updatedAt = committed.UpdatedAt().Add(time.Nanosecond)
	}
	replacement, err := current.Reference(plan.Document{Steps: steps, Source: doc.Source}, updatedAt)
	if err != nil {
		return err
	}
	change, err := plan.NewReplacement(current.Version(), replacement)
	if err != nil {
		return err
	}
	return store.Save(ctx, sessionID, change)
}
