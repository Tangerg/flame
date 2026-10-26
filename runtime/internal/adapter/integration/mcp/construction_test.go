package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

type constructionContext struct{ context.Context }

func TestOpenRejectsTypedNilCollaborators(t *testing.T) {
	for _, test := range []struct {
		name     string
		ctx      context.Context
		lifetime context.Context
		oauth    oauthSessionStore
		client   *sdkmcp.Client
	}{
		{"startup", (*constructionContext)(nil), t.Context(), nil, newClient()},
		{"lifetime", t.Context(), (*constructionContext)(nil), nil, newClient()},
		{"supplied oauth store", t.Context(), t.Context(), (*memoryOAuthStore)(nil), newClient()},
		{"client", t.Context(), t.Context(), nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			connections, err := openConnections(test.ctx, test.lifetime, nil, test.oauth, func(context.Context, mcpserver.ID) (*launchPermit, error) {
				t.Error("invalid construction reached configuration reads")
				return nil, errors.New("unexpected configuration read")
			}, nil, test.client)
			if connections != nil {
				if closeErr := connections.shutdown(context.WithoutCancel(t.Context())); closeErr != nil {
					t.Error(closeErr)
				}
			}
			if err == nil || connections != nil {
				t.Fatalf("invalid connections constructed: %v, %v", connections, err)
			}
		})
	}
}
