package persistence

import (
	"context"
	"errors"
	"fmt"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
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
	if _, err := resourceid.ParseSession(plan.SessionID); err != nil {
		return fmt.Errorf("persistence: conversation compaction: %w", err)
	}
	return c.tx(ctx, func(ctx context.Context) error { return c.applyCompaction(ctx, plan) })
}

func (c *ConversationCompactions) applyCompaction(
	ctx context.Context,
	plan runsapp.ConversationCompactionPlan,
) error {
	count, err := c.history.Count(ctx, plan.SessionID)
	if err != nil {
		return err
	}
	if count != plan.Compaction.ExpectedCount() {
		return fmt.Errorf(
			"persistence: conversation compaction message count changed from %d to %d",
			plan.Compaction.ExpectedCount(), count,
		)
	}
	current, err := c.runs.ListRuns(ctx, plan.SessionID)
	if err != nil {
		return err
	}
	if err := validateCompactionRuns(plan.SessionID, plan.Runs, current); err != nil {
		return err
	}
	if err := c.history.Replace(ctx, plan.SessionID, plan.Compaction.Messages()...); err != nil {
		return err
	}
	for _, planned := range plan.Runs {
		if planned.Expected().Equal(planned.State()) {
			continue
		}
		if err := c.runs.RebaseMessageMark(ctx, planned); err != nil {
			return err
		}
	}
	return nil
}

func validateCompactionRuns(
	sessionID string,
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
		if err := candidate.Validate(); err != nil {
			return fmt.Errorf("persistence: conversation compaction Run[%d]: %w", index, err)
		}
		expected := candidate.Expected()
		replacement := candidate.State()
		if !current[index].Equal(expected) {
			return fmt.Errorf("persistence: conversation compaction Run %q changed", expected.ID())
		}
		if expected.SessionID() != sessionID || replacement.SessionID() != sessionID {
			return fmt.Errorf("persistence: conversation compaction Run %q belongs to another session", expected.ID())
		}
	}
	return nil
}
