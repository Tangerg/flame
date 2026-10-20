package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/resourceid"
	"github.com/Tangerg/scope/core/chat"
)

var errConversationSessionIDRequired = errors.New("runs: conversation session ID is required")

// ConversationStore is the exact persistence capability consumed by conversation
// use cases, addressed by the Session that owns the conversation. Read transfers
// ownership of decoded messages to the caller. Write and Replace borrow messages
// only until return; retained values must be copied. Replace must atomically
// install the complete sequence.
//
// Write reports a batch whose effect the store could not observe by joining
// [ErrConversationWriteUncertain]: such a write must not be retried as though it
// had definitely been rejected.
type ConversationStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
	// Truncate keeps the first keepN messages and removes the rest as one
	// durable step, so retention neither reads the history back nor needs the
	// caller to hold a transaction for the two halves to agree.
	Truncate(ctx context.Context, sessionID string, keepN int) error
	Clear(ctx context.Context, sessionID string) error
}

// ErrConversationWriteUncertain marks an append whose durable effect is unknown.
// A caller that retries it may duplicate the batch, so it is a distinct fact
// from a write the store proved it rejected.
var ErrConversationWriteUncertain = errors.New("runs: conversation write outcome is unknown")

// ConversationHistory coordinates durable conversation operations while the
// domain value owns sequence validation and transformations.
type ConversationHistory struct{ store ConversationStore }

func NewConversationHistory(store ConversationStore) (*ConversationHistory, error) {
	if dependency.Missing(store) {
		return nil, errors.New("runs: conversation store is required")
	}
	return &ConversationHistory{store: store}, nil
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
	return messages, nil
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
	if err := conversation.ValidateMessages(messages); err != nil {
		return err
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("runs: inspect conversation seed target %q: %w", sessionID, err)
	}
	if count != 0 {
		return conversation.ErrNotEmpty
	}
	if err := m.store.Write(ctx, sessionID, messages...); err != nil {
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
	if err := conversation.ValidateMessages(messages); err != nil {
		return err
	}
	if err := m.store.Write(ctx, sessionID, messages...); err != nil {
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

// Truncate atomically keeps the first keepN messages. The store removes the
// tail in place: reading the history back to re-install a prefix would decode
// and rewrite every kept message, and would lose anything appended between the
// two halves unless a caller happened to hold a transaction around them.
func (m *ConversationHistory) Truncate(ctx context.Context, sessionID string, keepN int) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	keep := max(keepN, 0)
	if err := m.store.Truncate(ctx, sessionID, keep); err != nil {
		return fmt.Errorf("runs: truncate conversation for Session %q to %d messages: %w", sessionID, keep, err)
	}
	return nil
}

// Clear atomically removes every message without first decoding stored rows.
func (m *ConversationHistory) Clear(ctx context.Context, sessionID string) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	if err := m.store.Clear(ctx, sessionID); err != nil {
		return fmt.Errorf("runs: clear conversation for Session %q: %w", sessionID, err)
	}
	return nil
}

func validateConversationSessionIdentity(sessionID string) error {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return fmt.Errorf("%w: %v", errConversationSessionIDRequired, err)
	}
	return nil
}
