package mcp

import (
	"strings"
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

func TestMCPMutationResultsMustFulfillTheCommand(t *testing.T) {
	t.Parallel()
	timeout := mustHandshakeTimeout(t, 15)
	authorization := AuthorizationChange{Kind: protocol.MCPSecretSet, Value: "Bearer secret"}
	headers := HeadersChange{Kind: protocol.MCPSecretSet, Value: map[string]string{"X-Key": "secret"}}
	candidate := Candidate{
		Name: "docs", Enabled: true, Description: "Documentation", HandshakeTimeout: timeout,
		Connection: ConnectionInput{
			Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools",
			Authorization: &authorization, Headers: &headers,
		},
		DisabledTools: []string{"write"}, AutoApproveTools: []string{"search"},
	}
	valid := protocol.MCPServer{
		Name: candidate.Name, Description: candidate.Description, HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: new(15)},
		Connection: protocol.MCPConnection{
			Type: protocol.MCPTransportStreamableHTTP, URL: candidate.Connection.URL,
			AuthorizationMasked: "****", HeadersMasked: map[string]string{"X-Key": "****"},
		},
		DisabledTools: []string{"write"}, AutoApproveTools: []string{"search"},
		Status: protocol.MCPServerState{Type: protocol.MCPServerDisconnected},
	}
	if err := candidate.ValidateResult(valid); err != nil {
		t.Fatalf("valid create result: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*protocol.MCPServer)
		want   string
	}{
		{name: "description", mutate: func(result *protocol.MCPServer) { result.Description = "ignored" }, want: "description"},
		{name: "timeout", mutate: func(result *protocol.MCPServer) {
			result.HandshakeTimeout = protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: new(1)}
		}, want: "timeout"},
		{name: "URL", mutate: func(result *protocol.MCPServer) { result.Connection.URL = "https://other.example" }, want: "URL"},
		{name: "authorization", mutate: func(result *protocol.MCPServer) { result.Connection.AuthorizationMasked = "" }, want: "authorization"},
		{name: "headers", mutate: func(result *protocol.MCPServer) { result.Connection.HeadersMasked = nil }, want: "headers"},
		{name: "enabled", mutate: func(result *protocol.MCPServer) { result.Status.Type = protocol.MCPServerDisabled }, want: "enabled"},
		{name: "disabled tools", mutate: func(result *protocol.MCPServer) { result.DisabledTools = nil }, want: "disabled tools"},
	} {
		t.Run("create "+test.name, func(t *testing.T) {
			result := valid
			test.mutate(&result)
			err := candidate.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}

	description, enabled := "Updated", false
	updatedTimeout := mustHandshakeTimeout(t, 30)
	disabledTools := []string{"delete"}
	update := ServerUpdate{
		Server: candidate.Name, Enabled: &enabled, Description: &description,
		HandshakeTimeout: &updatedTimeout, DisabledTools: &disabledTools,
	}
	updated := valid
	updated.Description, updated.HandshakeTimeout = description, protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: new(30)}
	updated.DisabledTools, updated.Status = disabledTools, protocol.MCPServerState{Type: protocol.MCPServerDisabled}
	if err := update.ValidateResult(updated); err != nil {
		t.Fatalf("valid update result: %v", err)
	}
	wrongUpdate := updated
	wrongUpdate.Description = "ignored"
	if err := update.ValidateResult(wrongUpdate); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("update result error = %v", err)
	}

	clearAuthorization := AuthorizationChange{Kind: protocol.MCPSecretClear}
	clearHeaders := HeadersChange{Kind: protocol.MCPSecretClear}
	connectionUpdate := ServerUpdate{
		Server: candidate.Name,
		Connection: &ConnectionInput{
			Transport: protocol.MCPTransportStreamableHTTP, URL: candidate.Connection.URL,
			Authorization: &clearAuthorization, Headers: &clearHeaders,
		},
	}
	cleared := valid
	cleared.Connection.AuthorizationMasked = ""
	cleared.Connection.HeadersMasked = nil
	if err := connectionUpdate.ValidateResult(cleared); err != nil {
		t.Fatalf("valid secret clear result: %v", err)
	}
	uncleared := cleared
	uncleared.Connection.AuthorizationMasked = "****"
	if err := connectionUpdate.ValidateResult(uncleared); err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("secret clear result error = %v", err)
	}

	environment := EnvironmentChange{Kind: protocol.MCPSecretSet, Value: map[string]string{"TOKEN": "secret"}}
	stdioCandidate := Candidate{
		Name: "local", Enabled: false,
		Connection: ConnectionInput{
			Transport: protocol.MCPTransportStdio, Command: "mcp-server", Args: []string{"--stdio"},
			Environment: &environment, Directory: "/workspace",
		},
	}
	stdioResult := protocol.MCPServer{
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
		Name:             stdioCandidate.Name,
		Connection: protocol.MCPConnection{
			Type: protocol.MCPTransportStdio, Command: "mcp-server", Args: []string{"--stdio"},
			EnvMasked: map[string]string{"TOKEN": "****"}, Dir: "/workspace",
		},
		Status: protocol.MCPServerState{Type: protocol.MCPServerDisabled},
	}
	if err := stdioCandidate.ValidateResult(stdioResult); err != nil {
		t.Fatalf("valid stdio result: %v", err)
	}
	missingEnvironment := stdioResult
	missingEnvironment.Connection.EnvMasked = nil
	if err := stdioCandidate.ValidateResult(missingEnvironment); err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("stdio result error = %v", err)
	}
}

func TestMCPMutationResultsAcceptRuntimeToolPolicyCanonicalization(t *testing.T) {
	candidate := Candidate{
		Name: "docs", Enabled: true,
		Connection:       ConnectionInput{Transport: protocol.MCPTransportStdio, Command: "docs-server"},
		DisabledTools:    []string{"write", "read"},
		AutoApproveTools: []string{"search", "fetch"},
	}
	result := protocol.MCPServer{
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
		Name:             candidate.Name, Connection: protocol.MCPConnection{Type: protocol.MCPTransportStdio, Command: "docs-server"},
		DisabledTools: []string{"read", "write"}, AutoApproveTools: []string{"fetch", "search"},
		Status: protocol.MCPServerState{Type: protocol.MCPServerDisconnected},
	}
	if err := candidate.ValidateResult(result); err != nil {
		t.Fatalf("canonical create result: %v", err)
	}

	disabled := []string{"write", "read"}
	update := ServerUpdate{Server: "docs", DisabledTools: &disabled}
	if err := update.ValidateResult(result); err != nil {
		t.Fatalf("canonical update result: %v", err)
	}

	contradictory := candidate
	contradictory.AutoApproveTools = []string{"write"}
	if err := contradictory.Validate(); err == nil {
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
