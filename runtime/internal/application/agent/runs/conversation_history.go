package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

var errConversationSessionIDRequired = errors.New("runs: conversation session ID is required")

// Write and Replace borrow messages only until return. A batch whose effect the
// store could not observe joins ErrConversationWriteUncertain and must not be
// retried as if rejected.
type conversationStore interface {
	Read(ctx context.Context, sessionID identity.SessionID) ([]chat.Message, error)
	Write(ctx context.Context, sessionID identity.SessionID, messages ...chat.Message) error
	Count(ctx context.Context, sessionID identity.SessionID) (int, error)
	Replace(ctx context.Context, sessionID identity.SessionID, messages ...chat.Message) error
	Truncate(ctx context.Context, sessionID identity.SessionID, keepN int) error
	Clear(ctx context.Context, sessionID identity.SessionID) error
}

var ErrConversationWriteUncertain = errors.New("runs: conversation write outcome is unknown")

type ConversationHistory struct{ store conversationStore }

func NewConversationHistory(store conversationStore) (*ConversationHistory, error) {
	if lo.IsNil(store) {
		return nil, errors.New("runs: conversation store is required")
	}
	return &ConversationHistory{store: store}, nil
}

func (m *ConversationHistory) Read(ctx context.Context, sessionID identity.SessionID) ([]chat.Message, error) {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return nil, err
	}
	messages, err := m.store.Read(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runs: read conversation for Session %q: %w", sessionID, err)
	}
	return messages, nil
}

func (m *ConversationHistory) Seed(ctx context.Context, sessionID identity.SessionID, messages []chat.Message) error {
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

func (m *ConversationHistory) Append(ctx context.Context, sessionID identity.SessionID, messages ...chat.Message) error {
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

func (m *ConversationHistory) Count(ctx context.Context, sessionID identity.SessionID) (int, error) {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return 0, err
	}
	count, err := m.store.Count(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("runs: count conversation for Session %q: %w", sessionID, err)
	}
	return count, nil
}

func (m *ConversationHistory) Truncate(ctx context.Context, sessionID identity.SessionID, keepN int) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	keep := max(keepN, 0)
	if err := m.store.Truncate(ctx, sessionID, keep); err != nil {
		return fmt.Errorf("runs: truncate conversation for Session %q to %d messages: %w", sessionID, keep, err)
	}
	return nil
}

func (m *ConversationHistory) Clear(ctx context.Context, sessionID identity.SessionID) error {
	if err := validateConversationSessionIdentity(sessionID); err != nil {
		return err
	}
	if err := m.store.Clear(ctx, sessionID); err != nil {
		return fmt.Errorf("runs: clear conversation for Session %q: %w", sessionID, err)
	}
	return nil
}

func validateConversationSessionIdentity(sessionID identity.SessionID) error {
	if err := sessionID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errConversationSessionIDRequired, err)
	}
	return nil
}
