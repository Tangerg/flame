package persistence

import (
	"context"
	"fmt"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type ConversationCompactions struct {
	history *ConversationStore
	runs    *RunStore
	tx      Transactor
}

func (b *Bundle) ConversationCompactions() *ConversationCompactions {
	return &ConversationCompactions{history: b.Conversations, runs: b.Runs, tx: b.Transactor}
}

var _ runsapp.ConversationCompactionStore = (*ConversationCompactions)(nil)

func (c *ConversationCompactions) ApplyCompaction(ctx context.Context, plan runsapp.ConversationCompactionPlan) error {
	return c.tx(ctx, func(ctx context.Context) error { return c.applyCompaction(ctx, plan) })
}

func (c *ConversationCompactions) applyCompaction(
	ctx context.Context,
	plan runsapp.ConversationCompactionPlan,
) error {
	sessionID := plan.SessionID()
	compaction := plan.Compaction()
	planned := plan.Runs()
	count, err := c.history.Count(ctx, sessionID)
	if err != nil {
		return err
	}
	if count != compaction.ExpectedCount() {
		return fmt.Errorf(
			"persistence: conversation compaction message count changed from %d to %d",
			compaction.ExpectedCount(), count,
		)
	}
	current, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := validateCompactionRuns(planned, current); err != nil {
		return err
	}
	if err := c.history.Replace(ctx, sessionID, compaction.Messages()...); err != nil {
		return err
	}
	for _, replacement := range planned {
		if replacement.Expected().Equal(replacement.State()) {
			continue
		}
		if err := c.runs.RebaseMessageMark(ctx, replacement); err != nil {
			return err
		}
	}
	return nil
}

func validateCompactionRuns(
	planned []run.Replacement,
	current []run.Run,
) error {
	if len(current) != len(planned) {
		return fmt.Errorf(
			"persistence: conversation compaction Run set changed from %d to %d records",
			len(planned), len(current),
		)
	}
	for index, candidate := range planned {
		expected := candidate.Expected()
		if !current[index].Equal(expected) {
			return fmt.Errorf("persistence: conversation compaction Run %q changed", expected.ID())
		}
	}
	return nil
}
