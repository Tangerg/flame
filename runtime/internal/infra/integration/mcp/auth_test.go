package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/go-sdk/auth"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestFailedStatus(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantState   mcpserver.ConnectionState
		wantFailure mcpserver.ConnectionFailure
	}{
		{"typed auth rejection", errors.Join(mcpserver.ErrAuthorizationRequired, errors.New("connect rejected")), mcpserver.ConnectionNeedsAuth, ""},
		{"wrapped auth rejection", errors.Join(errors.New("dial failed"), errors.Join(mcpserver.ErrAuthorizationRequired, errors.New("connect rejected"))), mcpserver.ConnectionNeedsAuth, ""},
		{"401 text is not a type", errors.New("connect: server returned HTTP 401"), mcpserver.ConnectionFailed, mcpserver.FailureConnection},
		{"generic failure", errors.New("dial tcp: connection refused"), mcpserver.ConnectionFailed, mcpserver.FailureConnection},
		{"403 is not needsAuth", errors.New("HTTP 403 Forbidden"), mcpserver.ConnectionFailed, mcpserver.FailureConnection},
		{"nil", nil, mcpserver.ConnectionFailed, mcpserver.FailureConnection},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if state, failure := failedStatus(tc.err, mcpserver.FailureConnection); state != tc.wantState || failure != tc.wantFailure {
				t.Errorf("failedStatus(%v) = %q, %q, want %q, %q", tc.err, state, failure, tc.wantState, tc.wantFailure)
			}
		})
	}
}

// refusingInitializedServer accepts initialize, then answers the initialized
// notification with refusal. The dial still issues later requests (the session
// DELETE) whose statuses must not reclassify that refusal.
func refusingInitializedServer(t *testing.T, refusal int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var message struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if message.Method != "initialize" {
				w.WriteHeader(refusal)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-1")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"s","version":"1"}}}`, message.ID)
		case http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

type failingOAuthHandler struct{}

func (failingOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) { return nil, nil }

func (failingOAuthHandler) Authorize(_ context.Context, _ *http.Request, response *http.Response) error {
	return errors.Join(errors.New("authorization unavailable"), response.Body.Close())
}

func TestHTTPDialClassifiesTheRefusedRequestNotTheLastResponse(t *testing.T) {
	cases := []struct {
		name    string
		refusal int
		oauth   auth.OAuthHandler
		want    mcpserver.ConnectionState
	}{
		{"unauthorized", http.StatusUnauthorized, nil, mcpserver.ConnectionNeedsAuth},
		{"unauthorized with failed oauth", http.StatusUnauthorized, failingOAuthHandler{}, mcpserver.ConnectionNeedsAuth},
		{"forbidden", http.StatusForbidden, nil, mcpserver.ConnectionFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := refusingInitializedServer(t, tc.refusal)
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "v0"}, nil)
			_, _, err := dial(t.Context(), t.Context(), client, &launch{config: ServerConfig{
				Source:       mcpserver.UserSource(),
				Name:         testsupport.ServerName("refusing"),
				Transport:    TransportHTTP,
				Endpoint:     server.URL,
				OAuthHandler: tc.oauth,
			}})
			if err == nil {
				t.Fatal("dial succeeded, want a refused initialized notification")
			}
			if got, _ := failedStatus(err, mcpserver.FailureConnection); got != tc.want {
				t.Fatalf("dial status = %q, want %q; err = %v", got, tc.want, err)
			}
		})
	}
}
