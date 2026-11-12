package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type constructionContext struct{ context.Context }

func TestConnectionConstructionRejectsPointerVariantsBeforeCopying(t *testing.T) {
	for _, test := range []struct {
		name       string
		connection mcpserver.Connection
	}{
		{"nil_stdio", (*mcpserver.StdioConnection)(nil)},
		{"nil_http", (*mcpserver.HTTPConnection)(nil)},
		{"mutable_stdio", &mcpserver.StdioConnection{Command: "mcp-server"}},
		{"mutable_http", &mcpserver.HTTPConnection{URL: "https://mcp.example.test"}},
	} {
		server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("invalid"), Connection: test.connection}
		t.Run(test.name+"/connections", func(t *testing.T) {
			owner, err := newConnections(t.Context(), t.Context(), []ServerConfig{{Server: server}}, nil, nil)
			if owner != nil {
				if closeErr := owner.shutdown(context.WithoutCancel(t.Context())); closeErr != nil {
					t.Error(closeErr)
				}
			}
			if err == nil || owner != nil {
				t.Fatal("connection construction accepted a pointer variant")
			}
		})
		t.Run(test.name+"/pool", func(t *testing.T) {
			owner, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, &mutableSourceRegistry{}, nil, nil, nil, nil)
			if owner != nil {
				if closeErr := owner.Shutdown(context.WithoutCancel(t.Context())); closeErr != nil {
					t.Error(closeErr)
				}
			}
			if err == nil || owner != nil {
				t.Fatal("pool construction accepted a pointer variant")
			}
		})
	}
}

func TestConnectionProbeRejectsTypedNilBeforeCopying(t *testing.T) {
	for _, connection := range []mcpserver.Connection{(*mcpserver.StdioConnection)(nil), (*mcpserver.HTTPConnection)(nil)} {
		server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("invalid"), Connection: connection}
		if err := (&Pool{}).Probe(t.Context(), server); err == nil {
			t.Fatal("probe accepted a nil pointer variant")
		}
	}
}

func TestOpenRejectsTypedNilCollaborators(t *testing.T) {
	for _, test := range []struct {
		name     string
		ctx      context.Context
		lifetime context.Context
		oauth    oauthSessionStore
	}{
		{"startup", (*constructionContext)(nil), t.Context(), nil},
		{"lifetime", t.Context(), (*constructionContext)(nil), nil},
		{"supplied oauth store", t.Context(), t.Context(), (*memoryOAuthStore)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			connections, err := openConnections(test.ctx, test.lifetime, nil, test.oauth, func(context.Context, mcpserver.ID) (*launchPermit, error) {
				t.Error("invalid construction reached configuration reads")
				return nil, errors.New("unexpected configuration read")
			}, nil)
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
