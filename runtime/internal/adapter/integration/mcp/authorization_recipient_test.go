package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestAuthorizationUsesTheAdmittedRecipient(t *testing.T) {
	var contacts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)

	original := ServerConfig{Server: mcpserver.Server{
		Source: mcpserver.UserSource(), Name: testsupport.ServerName("recipient-fixture"), Enabled: true,
		Connection: mcpserver.HTTPConnection{URL: upstream.URL + "/original"},
	}}
	current := original.Clone()
	owner, err := newConnections(t.Context(), t.Context(), []ServerConfig{original}, &memoryOAuthStore{}, pluginmcp.NewCallbacks())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	recipient, eligible := original.OAuthTarget()
	if !eligible {
		t.Fatal("the original recipient is not OAuth eligible")
	}
	attempt, err := owner.beginAuthorize(t.Context(), original.ID(), recipient.Fingerprint(), func(context.Context, mcpserver.ID) (*launchPermit, error) {
		return newLaunch(current, nil, nil, admission{})
	})
	if err != nil {
		t.Fatal(err)
	}
	current.Connection = mcpserver.HTTPConnection{URL: upstream.URL + "/replacement"}
	err = attempt.Run()
	if contacts.Load() != 0 {
		t.Fatalf("authorization contacted a replacement recipient: %v", err)
	}
	if !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
		t.Fatalf("replaced recipient returned %v", err)
	}
}
