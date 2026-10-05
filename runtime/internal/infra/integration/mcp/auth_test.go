package mcp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
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

func TestHTTPTransportClassifiesObservedUnauthorizedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := endpointHTTPClient(server.URL, "", nil)
	if err != nil {
		t.Fatalf("endpointHTTPClient: %v", err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response: %v", err)
	}

	dialErr := classifyHTTPDialError(client, errors.New("SDK discarded response status"))
	if got, _ := failedStatus(dialErr, mcpserver.FailureConnection); got != mcpserver.ConnectionNeedsAuth {
		t.Fatalf("dial status = %q, want needsAuth", got)
	}
}
