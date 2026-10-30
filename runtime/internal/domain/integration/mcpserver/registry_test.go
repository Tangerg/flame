package mcpserver

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestServerValidateRejectsForgedBoundedHandshakeTimeout(t *testing.T) {
	srv := Server{
		Source:           UserSource(),
		Name:             testMCPServerName("linear"),
		Connection:       HTTPConnection{URL: "https://mcp.linear.app/mcp"},
		HandshakeTimeout: HandshakeTimeout{mode: handshakeTimeoutBounded, duration: -time.Second},
	}

	if err := srv.Validate(); err == nil {
		t.Fatal("Validate err = nil, want forged timeout rejected")
	}
}

func TestNewHandshakeTimeoutRequiresPositiveDuration(t *testing.T) {
	for _, duration := range []time.Duration{0, -time.Second, time.Millisecond, time.Second + time.Millisecond} {
		if _, err := NewHandshakeTimeout(duration); err == nil {
			t.Fatalf("NewHandshakeTimeout(%v) err = nil", duration)
		}
	}
}

func TestServerValidateRejectsInvalidHTTPEndpoints(t *testing.T) {
	for _, endpoint := range []string{"/mcp", "file:///tmp/mcp", "https://", "https://user:secret@example.test/mcp"} {
		t.Run(endpoint, func(t *testing.T) {
			server := Server{Source: UserSource(), Name: testMCPServerName("remote"), Connection: HTTPConnection{URL: endpoint}}
			if err := server.Validate(); err == nil {
				t.Fatal("invalid HTTP endpoint admitted as a server")
			}
		})
	}
}

func TestServerFormattingRedactsCredentials(t *testing.T) {
	for _, test := range []struct {
		connection Connection
		secrets    []string
		redactions int
	}{
		{
			connection: HTTPConnection{
				URL:           "https://url-user:url-secret@example.com/mcp",
				Authorization: "Bearer authorization-secret",
				Headers:       map[string]string{"X-Key": "header-secret"},
			},
			secrets:    []string{"url-user", "url-secret", "authorization-secret", "header-secret"},
			redactions: 3,
		},
		{
			connection: StdioConnection{Command: "server", Env: map[string]string{"TOKEN": "environment-secret"}},
			secrets:    []string{"environment-secret"},
			redactions: 1,
		},
	} {
		formatted := fmt.Sprintf("%+v", Server{Source: UserSource(), Name: testMCPServerName("private"), Connection: test.connection})
		for _, secret := range test.secrets {
			if strings.Contains(formatted, secret) {
				t.Fatalf("formatted server exposed %q: %s", secret, formatted)
			}
		}
		if got := strings.Count(formatted, "[REDACTED]"); got != test.redactions {
			t.Fatalf("formatted server redactions = %d, want %d: %s", got, test.redactions, formatted)
		}
	}
}

func TestServerCloneOwnsMutableConnectionFields(t *testing.T) {
	http := Server{Connection: HTTPConnection{Headers: map[string]string{"X-Key": "header"}}}
	process := Server{Connection: StdioConnection{Args: []string{"--root", "/repo"}, Env: map[string]string{"TOKEN": "environment"}}}

	httpClone := http.Clone().Connection.(HTTPConnection)
	httpClone.Headers["X-Key"] = "changed"
	processClone := process.Clone().Connection.(StdioConnection)
	processClone.Args[0] = "changed"
	processClone.Env["TOKEN"] = "changed"

	original := process.Connection.(StdioConnection)
	if http.Connection.(HTTPConnection).Headers["X-Key"] != "header" || original.Args[0] != "--root" || original.Env["TOKEN"] != "environment" {
		t.Fatal("Clone mutated the original connection")
	}
}
