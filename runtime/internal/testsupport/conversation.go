// Package testsupport provides shared Runtime test builders and in-memory
// fakes. Production code must use semantic Domain and Application paths.
package testsupport

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
	"github.com/Tangerg/scope/core/history/inmemory"
)

// ConversationStore doubles the Session-keyed conversation port over Scope's
// in-memory history store, so a fixture cannot drift from the storage semantics
// production depends on. Only the retention capabilities Runtime adds beyond
// Scope's contract are implemented here.
type ConversationStore struct {
	messages *inmemory.Store
}

// NewConversationStore returns an empty app-port-compatible conversation store.
func NewConversationStore() *ConversationStore {
	return &ConversationStore{messages: new(inmemory.Store)}
}

// Read returns the messages stored for sessionID.
func (s *ConversationStore) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.messages.Read(ctx, id)
}

// Write appends messages to sessionID.
func (s *ConversationStore) Write(ctx context.Context, sessionID string, messages ...chat.Message) error {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	outcome, err := s.messages.Write(ctx, id, messages...)
	if validateErr := outcome.Validate(len(messages), err); validateErr != nil {
		return validateErr
	}
	return err
}

// Clear removes sessionID's messages.
func (s *ConversationStore) Clear(ctx context.Context, sessionID string) error {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.messages.Clear(ctx, id)
}

// Replace atomically sets sessionID's messages.
func (s *ConversationStore) Replace(ctx context.Context, sessionID string, messages ...chat.Message) error {
	if err := s.Clear(ctx, sessionID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	return s.Write(ctx, sessionID, messages...)
}

// Truncate keeps sessionID's first keepN messages.
func (s *ConversationStore) Truncate(ctx context.Context, sessionID string, keepN int) error {
	if keepN < 0 {
		return fmt.Errorf("conversation fixture: keep count %d is negative", keepN)
	}
	stored, err := s.Read(ctx, sessionID)
	if err != nil {
		return err
	}
	if keepN >= len(stored) {
		return nil
	}
	return s.Replace(ctx, sessionID, stored[:keepN]...)
}

// Count returns sessionID's message count.
func (s *ConversationStore) Count(ctx context.Context, sessionID string) (int, error) {
	stored, err := s.Read(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	return len(stored), nil
}

// conversationID admits a Session as the conversation it owns, which is also
// where the fixture honors cancellation.
func conversationID(ctx context.Context, sessionID string) (history.ConversationID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id := history.ConversationID(sessionID)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}
