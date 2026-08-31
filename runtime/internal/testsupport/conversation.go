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

// ConversationStore is Runtime's test-only in-memory implementation of its session-ID
// conversation ports. Production uses the SQLite MessageStore directly.
type ConversationStore struct {
	mu       sync.RWMutex
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMessages(s.messages[id])
}

// Write appends messages to sessionID.
func (s *ConversationStore) Write(ctx context.Context, sessionID string, messages ...chat.Message) error {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return err
	}
	snapshot, err := cloneMessages(messages)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = append(s.messages[id], snapshot...)
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
	snapshot, err := cloneMessages(messages)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(snapshot) == 0 {
		delete(s.messages, id)
	} else {
		s.messages[id] = snapshot
	}
	return nil
}

// Count returns sessionID's message count.
func (s *ConversationStore) Count(ctx context.Context, sessionID string) (int, error) {
	id, err := conversationID(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[id]), nil
}

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

func cloneMessages(messages []chat.Message) ([]chat.Message, error) {
	cloned := make([]chat.Message, len(messages))
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("conversation fixture: messages[%d]: %w", index, err)
		}
		cloned[index] = messages[index].Clone()
	}
	return cloned, nil
}
