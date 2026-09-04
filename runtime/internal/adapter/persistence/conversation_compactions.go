package persistence

import (
	"context"
	"errors"
	"fmt"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

type conversationHistory interface {
	Count(ctx context.Context, sessionID string) (int, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
}

type conversationRuns interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
	RebaseMessageMark(ctx context.Context, replacement run.Replacement) error
}

// ConversationCompactions applies an Application-decided conversation rewrite
// and all of its Run-watermark replacements in one storage transaction.
type ConversationCompactions struct {
	history conversationHistory
	runs    conversationRuns
	tx      Transactor
}

func NewConversationCompactions(history conversationHistory, runs conversationRuns, tx Transactor) *ConversationCompactions {
	return &ConversationCompactions{history: history, runs: runs, tx: tx}
}

var _ runsapp.ConversationCompactionStore = (*ConversationCompactions)(nil)

func (c *ConversationCompactions) ListRuns(ctx context.Context, sessionID string) ([]run.Run, error) {
	if c == nil || c.runs == nil {
		return nil, errors.New("persistence: conversation compaction Run store is unavailable")
	}
	return c.runs.ListRuns(ctx, sessionID)
}

func (c *ConversationCompactions) ApplyCompaction(ctx context.Context, plan runsapp.ConversationCompactionPlan) error {
	if c == nil || c.history == nil || c.runs == nil || c.tx == nil {
		return errors.New("persistence: conversation compaction dependencies are unavailable")
	}
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("persistence: conversation compaction: %w", err)
	}
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
