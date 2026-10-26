package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
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
	seen := make(map[string]struct{}, len(runs))
	for index, replacement := range runs {
		if err := replacement.Validate(); err != nil {
			return ConversationCompactionPlan{}, fmt.Errorf("runs: conversation compaction run[%d]: %w", index, err)
		}
		expected := replacement.Expected()
		if expected.SessionID() != id.String() {
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
		sessionID: id, compaction: compaction, runs: slices.Clone(runs),
	}, nil
}

// SessionID returns the exact conversation owner.
func (p ConversationCompactionPlan) SessionID() string { return p.sessionID.String() }

// Compaction returns the validated coordinate transformation.
func (p ConversationCompactionPlan) Compaction() conversation.Compaction { return p.compaction }

// Runs returns an isolated snapshot of the exact Run replacements.
func (p ConversationCompactionPlan) Runs() []run.Replacement { return slices.Clone(p.runs) }

// ConversationCompactionStore joins the decided rewrite to its owning event transaction.
type ConversationCompactionStore interface {
	ApplyCompaction(context.Context, ConversationCompactionPlan) error
}

func PlanConversationCompaction(sessionID string, compaction conversation.Compaction, current []run.Run) (ConversationCompactionPlan, error) {
	planned := make([]run.Replacement, len(current))
	for index, value := range current {
		replacement, err := run.Replace(value, func(value run.Run) (run.Run, error) {
			if !value.State().IsTerminal() || !value.Lineage().IsRoot() {
				return value, nil
			}
			count, known := value.MessageMark().Count()
			if !known {
				return run.Run{}, errors.New("terminal root Run has no message watermark")
			}
			mark, err := compaction.RebaseMessageMark(count)
			if err != nil {
				return run.Run{}, err
			}
			return value.WithMessageMark(mark)
		})
		if err != nil {
			return ConversationCompactionPlan{}, fmt.Errorf("runs: rebase conversation Run %q: %w", value.ID(), err)
		}
		planned[index] = replacement
	}
	return NewConversationCompactionPlan(sessionID, compaction, planned)
}

func (p ConversationCompactionPlan) validateOwner(runID, segmentID string) error {
	for _, replacement := range p.runs {
		owner := replacement.Expected()
		if owner.ID() != runID {
			continue
		}
		if owner.State() == run.Running && owner.Lineage().IsRoot() && owner.ActiveSegmentID() == segmentID {
			return nil
		}
		return errors.New("runs: conversation compaction requires its active root Segment")
	}
	return errors.New("runs: conversation compaction omits its Run owner")
}
