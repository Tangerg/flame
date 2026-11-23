package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
)

// Runs includes unchanged non-terminal records so persistence can reject a
// lifecycle transition that raced the plan.
type ConversationCompactionPlan struct {
	sessionID  identity.SessionID
	compaction conversation.Compaction
	runs       []run.Replacement
}

func newConversationCompactionPlan(
	sessionID identity.SessionID,
	compaction conversation.Compaction,
	runs []run.Replacement,
) (ConversationCompactionPlan, error) {
	if err := sessionID.Validate(); err != nil {
		return ConversationCompactionPlan{}, fmt.Errorf("runs: conversation compaction session: %w", err)
	}
	seen := make(map[identity.RunID]struct{}, len(runs))
	for index, replacement := range runs {
		if err := replacement.Validate(); err != nil {
			return ConversationCompactionPlan{}, fmt.Errorf("runs: conversation compaction run[%d]: %w", index, err)
		}
		expected := replacement.Expected()
		if expected.SessionID() != sessionID {
			return ConversationCompactionPlan{}, fmt.Errorf(
				"runs: conversation compaction run %q belongs to another session",
				expected.ID(),
			)
		}
		if _, duplicate := seen[expected.ID()]; duplicate {
			return ConversationCompactionPlan{}, fmt.Errorf("runs: conversation compaction repeats run %q", expected.ID())
		}
		seen[expected.ID()] = struct{}{}
	}
	return ConversationCompactionPlan{
		sessionID: sessionID, compaction: compaction, runs: slices.Clone(runs),
	}, nil
}

func (p ConversationCompactionPlan) SessionID() identity.SessionID { return p.sessionID }

func (p ConversationCompactionPlan) Compaction() conversation.Compaction { return p.compaction }

func (p ConversationCompactionPlan) Runs() []run.Replacement { return slices.Clone(p.runs) }

type ConversationCompactionStore interface {
	ApplyCompaction(context.Context, ConversationCompactionPlan) error
}

func PlanConversationCompaction(sessionID identity.SessionID, compaction conversation.Compaction, current []run.Run) (ConversationCompactionPlan, error) {
	planned := make([]run.Replacement, len(current))
	for index, value := range current {
		replacement, err := value.Replace().RebaseConversation(compaction)
		if err != nil {
			return ConversationCompactionPlan{}, fmt.Errorf("runs: rebase conversation Run %q: %w", value.ID(), err)
		}
		planned[index] = replacement
	}
	return newConversationCompactionPlan(sessionID, compaction, planned)
}

func (p ConversationCompactionPlan) validateOwner(runID identity.RunID, segmentID identity.SegmentID) error {
	for _, replacement := range p.runs {
		owner := replacement.Expected()
		if owner.ID() != runID {
			continue
		}
		if owner.Lineage().IsRoot() && owner.Executes(segmentID) {
			return nil
		}
		return errors.New("runs: conversation compaction requires its active root Segment")
	}
	return errors.New("runs: conversation compaction omits its Run owner")
}
