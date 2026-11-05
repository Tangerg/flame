package persistence

import (
	"context"
	"errors"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/frame/sqlite"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
	"github.com/samber/lo"
)

// conversationMessages is Flame's mutable, per-session history. Compaction
// and rollback replace its contents; Scope's append-only batch store owns a
// different contract and is not implemented by this adapter.
type conversationMessages interface {
	Read(context.Context, history.ConversationID) ([]chat.Message, error)
	Write(context.Context, history.ConversationID, ...chat.Message) error
	Clear(context.Context, history.ConversationID) error
	Count(ctx context.Context, conversationID history.ConversationID) (int, error)
	Replace(ctx context.Context, conversationID history.ConversationID, messages ...chat.Message) error
	Truncate(ctx context.Context, conversationID history.ConversationID, keepN int) error
}

// ConversationStore adapts durable conversation history to the Session-keyed
// port the Run use cases consume. It owns the two translations that boundary
// needs: a Session names exactly one conversation, and a batch whose durable
// effect the store could not observe is reported as uncertain rather than as a
// definite rejection the caller is free to retry.
type ConversationStore struct {
	messages conversationMessages
}

// NewConversationStore binds the conversation use cases to a history store.
func NewConversationStore(messages conversationMessages) (*ConversationStore, error) {
	if lo.IsNil(messages) {
		return nil, errors.New("persistence: conversation history store is required")
	}
	return &ConversationStore{messages: messages}, nil
}

func (c *ConversationStore) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return c.messages.Read(ctx, history.ConversationID(sessionID))
}

func (c *ConversationStore) Write(ctx context.Context, sessionID string, messages ...chat.Message) error {
	err := c.messages.Write(ctx, history.ConversationID(sessionID), messages...)
	if errors.Is(err, sqlite.ErrIndeterminate) {
		return errors.Join(runsapp.ErrConversationWriteUncertain, err)
	}
	return err
}

func (c *ConversationStore) Count(ctx context.Context, sessionID string) (int, error) {
	return c.messages.Count(ctx, history.ConversationID(sessionID))
}

func (c *ConversationStore) Replace(ctx context.Context, sessionID string, messages ...chat.Message) error {
	return c.messages.Replace(ctx, history.ConversationID(sessionID), messages...)
}

func (c *ConversationStore) Truncate(ctx context.Context, sessionID string, keepN int) error {
	return c.messages.Truncate(ctx, history.ConversationID(sessionID), keepN)
}

func (c *ConversationStore) Clear(ctx context.Context, sessionID string) error {
	return c.messages.Clear(ctx, history.ConversationID(sessionID))
}
