package session

import (
	"context"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

type updater interface {
	UpdateSession(context.Context, conversation.UpdateSession) (conversation.Session, error)
}

// Update executes one optimistic session mutation and verifies that the
// runtime response fulfills the exact command before consumers project it.
func Update(ctx context.Context, writer updater, update conversation.UpdateSession) (conversation.Session, error) {
	if err := update.Validate(); err != nil {
		return conversation.Session{}, err
	}
	updated, err := writer.UpdateSession(ctx, update)
	if err != nil {
		return conversation.Session{}, err
	}
	if err := update.ValidateResult(updated); err != nil {
		return conversation.Session{}, err
	}
	return updated, nil
}
