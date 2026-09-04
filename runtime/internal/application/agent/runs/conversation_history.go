package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/scope/core/chat"
)

var errConversationSessionIDRequired = errors.New("runs: conversation session ID is required")

// ConversationStore is the exact persistence capability consumed by conversation use
// cases. Replace must atomically install the complete sequence.
type ConversationStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
}

// ConversationCompactionStore is the exact persistence capability for coordinate-changing
// conversation rewrites. Reading Runs and applying the decided replacement are
// separate because summary generation must happen outside a database
// transaction; ApplyCompaction rechecks the complete snapshot atomically.
type ConversationCompactionStore interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
	ApplyCompaction(ctx context.Context, plan ConversationCompactionPlan) error
}

// ConversationHistory coordinates durable conversation operations while the domain value
// owns sequence validation and transformations.
type ConversationHistory struct {
	store       ConversationStore
	compactions ConversationCompactionStore
}

// NewConversationHistory returns the conversation use cases backed by store.
func NewConversationHistory(store ConversationStore, compactions ConversationCompactionStore) *ConversationHistory {
	return &ConversationHistory{store: store, compactions: compactions}
}

// Read returns the validated durable conversation snapshot.
func (m *ConversationHistory) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return nil, err
	}
	messages, err := m.store.Read(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runs: read conversation for Session %q: %w", sessionID, err)
	}
	history, err := conversation.New(messages)
	if err != nil {
		return nil, fmt.Errorf("runs: validate conversation for Session %q: %w", sessionID, err)
	}
	return history.Messages(), nil
}

// Seed installs a prefix into a fresh conversation. Existing history is never
// silently appended to or replaced by a fork/import operation.
func (m *ConversationHistory) Seed(ctx context.Context, sessionID string, messages []chat.Message) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	seeded, err := (conversation.Conversation{}).Seed(messages)
	if err != nil {
		return err
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("runs: inspect conversation seed target %q: %w", sessionID, err)
	}
	if count != 0 {
		return conversation.ErrNotEmpty
	}
	if err := m.store.Write(ctx, sessionID, seeded.Messages()...); err != nil {
		return fmt.Errorf("runs: seed conversation for Session %q: %w", sessionID, err)
	}
	return nil
}

// Append extends an existing conversation with validated model-context messages.
func (m *ConversationHistory) Append(ctx context.Context, sessionID string, messages ...chat.Message) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	addition, err := conversation.New(messages)
	if err != nil {
		return err
	}
	stored, err := m.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	history, err := conversation.New(stored)
	if err != nil {
		return err
	}
	extended, err := history.Append(addition.Messages()...)
	if err != nil {
		return err
	}
	if err := m.store.Write(ctx, sessionID, extended.Messages()[len(stored):]...); err != nil {
		return fmt.Errorf("runs: append conversation for Session %q: %w", sessionID, err)
	}
	return nil
}

// Count returns the durable message watermark.
func (m *ConversationHistory) Count(ctx context.Context, sessionID string) (int, error) {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return 0, err
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("runs: count conversation for Session %q: %w", sessionID, err)
	}
	return count, nil
}

// RewriteForCompaction installs a summary or content-trim replacement and
// rebases every terminal Run watermark into its new coordinate space. The
// history and Run projection commit as one persistence write-set; a stale
// message count or Run snapshot fails the complete operation.
func (m *ConversationHistory) RewriteForCompaction(
	ctx context.Context,
	sessionID string,
	expectedCount int,
	cutoff int,
	replacementPrefix int,
	messages ...chat.Message,
) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	if m.compactions == nil {
		return errors.New("runs: conversation compaction persistence is unavailable")
	}
	compaction, err := conversation.NewCompaction(expectedCount, cutoff, replacementPrefix, messages)
	if err != nil {
		return err
	}
	runs, err := m.compactions.ListRuns(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("runs: read conversation compaction Runs for Session %q: %w", sessionID, err)
	}
	planned := make([]run.Replacement, len(runs))
	for index, current := range runs {
		replacement := current
		if current.State().IsTerminal() {
			mark, err := compaction.RebaseMessageMark(current.MessageMark())
			if err != nil {
				return fmt.Errorf("runs: rebase conversation Run %q: %w", current.ID(), err)
			}
			replacement, err = current.WithMessageMark(mark)
			if err != nil {
				return fmt.Errorf("runs: rebase conversation Run %q: %w", current.ID(), err)
			}
		}
		planned[index], err = run.NewReplacement(current, replacement)
		if err != nil {
			return fmt.Errorf("runs: prepare conversation Run %q replacement: %w", current.ID(), err)
		}
	}
	plan, err := NewConversationCompactionPlan(sessionID, compaction, planned)
	if err != nil {
		return fmt.Errorf("runs: prepare conversation compaction for Session %q: %w", sessionID, err)
	}
	if err := m.compactions.ApplyCompaction(ctx, plan); err != nil {
		return fmt.Errorf("runs: compact conversation for Session %q: %w", sessionID, err)
	}
	return nil
}

// Truncate atomically keeps the first keepN messages.
func (m *ConversationHistory) Truncate(ctx context.Context, sessionID string, keepN int) error {
	stored, err := m.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	history, err := conversation.New(stored)
	if err != nil {
		return err
	}
	if keepN >= history.Count() {
		return nil
	}
	if err := m.store.Replace(ctx, sessionID, history.Truncate(keepN).Messages()...); err != nil {
		return fmt.Errorf("runs: truncate conversation for Session %q to %d messages: %w", sessionID, max(keepN, 0), err)
	}
	return nil
}

// Clear atomically removes every message without first decoding stored rows.
func (m *ConversationHistory) Clear(ctx context.Context, sessionID string) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	if err := m.store.Replace(ctx, sessionID); err != nil {
		return fmt.Errorf("runs: clear conversation for Session %q: %w", sessionID, err)
	}
	return nil
}

func validateConversationSessionIdentity(sessionID string) error {
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return fmt.Errorf("%w: %v", errConversationSessionIDRequired, err)
	}
	return nil
}
