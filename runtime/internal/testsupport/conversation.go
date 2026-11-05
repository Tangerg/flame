// Package testsupport provides shared Runtime test builders and in-memory
// fakes. Production code must use semantic Domain and Application paths.
package testsupport

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
)

// ConversationStore is a synchronized test double for Flame's mutable
// Session-keyed conversation port, including atomic retention operations.
type ConversationStore struct {
	mu       sync.Mutex
	messages map[history.ConversationID][]chat.Message
}

// NewConversationStore returns an empty app-port-compatible conversation store.
func NewConversationStore() *ConversationStore {
	return &ConversationStore{messages: make(map[history.ConversationID][]chat.Message)}
}

// Read returns the messages stored for sessionID.
func (s *ConversationStore) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneConversation(s.messages[id]), nil
}

// Write appends messages to sessionID.
func (s *ConversationStore) Write(ctx context.Context, sessionID string, messages ...chat.Message) error {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = append(s.messages[id], cloneConversation(messages)...)
	return nil
}

// Clear removes sessionID's messages.
func (s *ConversationStore) Clear(ctx context.Context, sessionID string) error {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, id)
	return nil
}

// Replace atomically sets sessionID's messages.
func (s *ConversationStore) Replace(ctx context.Context, sessionID string, messages ...chat.Message) error {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = cloneConversation(messages)
	return nil
}

// Truncate keeps sessionID's first keepN messages.
func (s *ConversationStore) Truncate(ctx context.Context, sessionID string, keepN int) error {
	if keepN < 0 {
		return fmt.Errorf("conversation fixture: keep count %d is negative", keepN)
	}
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.messages[id]
	if keepN < len(stored) {
		s.messages[id] = cloneConversation(stored[:keepN])
	}
	return nil
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

func cloneConversation(messages []chat.Message) []chat.Message {
	cloned := make([]chat.Message, len(messages))
	for index, message := range messages {
		cloned[index] = message.Clone()
	}
	return cloned
}
