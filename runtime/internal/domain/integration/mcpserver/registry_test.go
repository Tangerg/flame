package mcpserver

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestServerValidateRejectsForgedBoundedHandshakeTimeout(t *testing.T) {
	srv := Server{
		Name:             testMCPServerName("linear"),
		Transport:        TransportStreamableHTTP,
		URL:              "https://mcp.linear.app/mcp",
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

func TestServerValidateRejectsCrossTransportState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Server)
	}{
		{name: "command", mutate: func(server *Server) { server.Command = "node" }},
		{name: "args", mutate: func(server *Server) { server.Args = []string{"server.js"} }},
		{name: "environment", mutate: func(server *Server) { server.Env = map[string]string{"TOKEN": "secret"} }},
		{name: "working directory", mutate: func(server *Server) { server.Dir = "/repo" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := Server{
				Name: testMCPServerName("cloud"), Transport: TransportStreamableHTTP, URL: "https://example.com/mcp",
			}
			test.mutate(&server)
			if err := server.Validate(); err == nil {
				t.Fatal("Validate err = nil, want cross-transport state rejected")
			}
		})
	}
}

func TestServerFormattingRedactsCredentials(t *testing.T) {
	server := Server{
		Name:          testMCPServerName("private"),
		Transport:     TransportStreamableHTTP,
		URL:           "https://url-user:url-secret@example.com/mcp",
		Authorization: "Bearer authorization-secret",
		Headers:       map[string]string{"X-Key": "header-secret"},
		Env:           map[string]string{"TOKEN": "environment-secret"},
	}

	formatted := fmt.Sprintf("%+v", server)
	for _, secret := range []string{"url-user", "url-secret", "authorization-secret", "header-secret", "environment-secret"} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("formatted server exposed %q: %s", secret, formatted)
		}
	}
	if got := strings.Count(formatted, "[REDACTED]"); got != 4 {
		t.Fatalf("formatted server redactions = %d, want 4: %s", got, formatted)
	}
}
