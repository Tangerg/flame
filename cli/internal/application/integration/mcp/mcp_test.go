package mcp

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestConnectionInputsKeepTransportAndSecretScopesClosed(t *testing.T) {
	authorization := AuthorizationChange{Kind: protocol.MCPSecretSet, Value: "Bearer secret"}
	http := ConnectionInput{Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools", Authorization: &authorization}
	if err := http.Validate(); err != nil {
		t.Fatal(err)
	}
	http.Command = "server"
	if err := http.Validate(); err == nil {
		t.Fatal("HTTP connection carrying a stdio command was accepted")
	}
	environment := EnvironmentChange{Kind: protocol.MCPSecretSet, Value: map[string]string{"TOKEN": "secret"}}
	stdio := ConnectionInput{Transport: protocol.MCPTransportStdio, Command: "server", Environment: &environment}
	if err := stdio.Validate(); err != nil {
		t.Fatal(err)
	}
	stdio.Authorization = &authorization
	if err := stdio.Validate(); err == nil {
		t.Fatal("stdio connection carrying HTTP authorization was accepted")
	}
	clear := AuthorizationChange{Kind: protocol.MCPSecretClear}
	candidate := Candidate{Name: "docs", Connection: ConnectionInput{Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example", Authorization: &clear}}
	if err := candidate.Validate(); err == nil {
		t.Fatal("candidate clearing a nonexistent secret was accepted")
	}
}

func TestServerAndAuthorizationStatesRejectContradictoryData(t *testing.T) {
	count := 2
	server := protocol.MCPServer{
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
		Name:             "docs", Connection: protocol.MCPConnection{Type: protocol.MCPTransportStdio, Command: "docs-server"},
		Status: protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &count},
	}
	if err := ValidateServer(server); err != nil {
		t.Fatal(err)
	}
	server.Status.Error = &protocol.ProblemData{Type: "mcp_dial_failed"}
	if err := ValidateServer(server); err == nil {
		t.Fatal("connected state carrying a problem was accepted")
	}
	server.Status.Error = nil
	server.DisabledTools = []string{"write"}
	server.AutoApproveTools = []string{"write"}
	if err := ValidateServer(server); err == nil {
		t.Fatal("server accepted contradictory tool policy")
	}
	now := time.Now()
	attempt := protocol.MCPAuthorizationAttempt{
		ID: "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA", Server: "docs",
		Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending}, CreatedAt: now,
	}
	if err := ValidateAuthorizationAttempt(attempt); err != nil {
		t.Fatal(err)
	}
	attempt.CreatedAt = time.Time{}
	if err := ValidateAuthorizationAttempt(attempt); err == nil {
		t.Fatal("authorization without creation time was accepted")
	}
	attempt.CreatedAt = now
	attempt.Status.Type = protocol.MCPAuthorizationAttemptFailed
	if err := ValidateAuthorizationAttempt(attempt); err == nil {
		t.Fatal("failed authorization without terminal data was accepted")
	}
}

func TestServerUpdateRequiresAnExplicitChange(t *testing.T) {
	if err := (ServerUpdate{Server: "docs"}).Validate(); err == nil {
		t.Fatal("empty MCP update was accepted")
	}
	description := "Documentation tools"
	if err := (ServerUpdate{Server: "docs", Description: &description}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateRejectsContradictoryToolPolicy(t *testing.T) {
	candidate := Candidate{
		Name: "docs", Enabled: true,
		Connection:    ConnectionInput{Transport: protocol.MCPTransportStdio, Command: "docs-server"},
		DisabledTools: []string{"write"}, AutoApproveTools: []string{"write"},
	}
	if err := candidate.Validate(); err == nil {
		t.Fatal("candidate accepted contradictory tool policy")
	}
}

func mustHandshakeTimeout(t *testing.T, seconds int) HandshakeTimeout {
	t.Helper()
	timeout, err := NewHandshakeTimeout(seconds)
	if err != nil {
		t.Fatalf("NewHandshakeTimeout(%d): %v", seconds, err)
	}
	return timeout
}

func TestHandshakeTimeoutRejectsNumericDisableSentinel(t *testing.T) {
	for _, seconds := range []int{0, -1} {
		if _, err := NewHandshakeTimeout(seconds); err == nil {
			t.Fatalf("NewHandshakeTimeout(%d) accepted", seconds)
		}
	}
	if err := (HandshakeTimeout{}).Validate(); err != nil {
		t.Fatalf("explicit unbounded zero value rejected: %v", err)
	}
}
