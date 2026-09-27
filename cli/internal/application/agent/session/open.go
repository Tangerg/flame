// Package session owns CLI application use cases and consumer contracts around
// durable Runtime Sessions.
package session

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

type runtime interface {
	CreateSession(context.Context, conversation.CreateSession) (conversation.Session, error)
	GetSession(context.Context, string) (conversation.SessionSnapshot, error)
}

// Open restores the selected session or creates a new one in workspace.
func Open(ctx context.Context, rt runtime, id, workspace string) (conversation.SessionSnapshot, error) {
	if id != "" {
		snapshot, err := rt.GetSession(ctx, id)
		if err != nil {
			return conversation.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		if err := snapshot.Validate(); err != nil {
			return conversation.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		return snapshot, nil
	}

	created, err := rt.CreateSession(ctx, conversation.CreateSession{Workspace: workspace})
	if err != nil {
		return conversation.SessionSnapshot{}, fmt.Errorf("create session: %w", err)
	}
	snapshot := conversation.SessionSnapshot{Session: created}
	if err := snapshot.Validate(); err != nil {
		return conversation.SessionSnapshot{}, fmt.Errorf("create session: %w", err)
	}
	return snapshot, nil
}
