package runs

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
)

// ConversationCompactionPlan is the complete cross-aggregate write-set for one
// history rewrite. Runs includes non-terminal records unchanged so persistence
// can reject a lifecycle transition that raced the plan instead of committing
// history and Run projections derived from different snapshots.
type ConversationCompactionPlan struct {
	sessionID  resourceid.SessionID
	compaction conversation.Compaction
	runs       []run.Replacement
}

// NewConversationCompactionPlan binds one conversation rewrite to the exact
// same-Session Run set from which its watermark replacements were derived.
func NewConversationCompactionPlan(
	sessionID string,
	compaction conversation.Compaction,
	runs []run.Replacement,
) (ConversationCompactionPlan, error) {
	id, err := resourceid.ParseSession(sessionID)
	if err != nil {
		return ConversationCompactionPlan{}, fmt.Errorf("runs: conversation compaction session: %w", err)
	}
	plan := ConversationCompactionPlan{
		sessionID: id, compaction: compaction, runs: slices.Clone(runs),
	}
	if err := plan.Validate(); err != nil {
		return ConversationCompactionPlan{}, err
	}
	return plan, nil
}

// Validate proves the write-set contains valid, uniquely identified Run
// replacements belonging to its one conversation Session.
func (p ConversationCompactionPlan) Validate() error {
	if err := p.sessionID.Validate(); err != nil {
		return fmt.Errorf("runs: conversation compaction session: %w", err)
	}
	seen := make(map[string]struct{}, len(p.runs))
	for index, replacement := range p.runs {
		if err := replacement.Validate(); err != nil {
			return fmt.Errorf("runs: conversation compaction run[%d]: %w", index, err)
		}
		expected := replacement.Expected()
		if expected.SessionID() != p.sessionID.String() {
			return fmt.Errorf(
				"runs: conversation compaction run %q belongs to another session",
				expected.ID(),
			)
		}
		if _, duplicate := seen[expected.ID()]; duplicate {
			return fmt.Errorf("runs: conversation compaction repeats run %q", expected.ID())
		}
		seen[expected.ID()] = struct{}{}
	}
	return nil
}

// SessionID returns the exact conversation owner.
func (p ConversationCompactionPlan) SessionID() string { return p.sessionID.String() }

// Compaction returns the validated coordinate transformation.
func (p ConversationCompactionPlan) Compaction() conversation.Compaction { return p.compaction }

// Runs returns an isolated snapshot of the exact Run replacements.
func (p ConversationCompactionPlan) Runs() []run.Replacement { return slices.Clone(p.runs) }
