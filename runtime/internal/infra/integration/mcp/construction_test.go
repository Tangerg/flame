package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type constructionContext struct{ context.Context }

func TestDialRejectsTypedNilCollaborators(t *testing.T) {
	for _, test := range []struct {
		name     string
		ctx      context.Context
		lifetime context.Context
		oauth    OAuthSessionStore
	}{
		{"startup", (*constructionContext)(nil), t.Context(), nil},
		{"lifetime", t.Context(), (*constructionContext)(nil), nil},
		{"supplied oauth store", t.Context(), t.Context(), (*memoryOAuthStore)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			connections, err := Dial(test.ctx, test.lifetime, nil, test.oauth, func(context.Context, mcpserver.ID) (*Launch, error) {
				t.Error("invalid dial reached configuration reads")
				return nil, errors.New("unexpected configuration read")
			})
			if connections != nil {
				if closeErr := connections.Shutdown(context.WithoutCancel(t.Context())); closeErr != nil {
					t.Error(closeErr)
				}
			}
			if err == nil || connections != nil {
				t.Fatalf("invalid connections constructed: %v, %v", connections, err)
			}
		})
	}
}
