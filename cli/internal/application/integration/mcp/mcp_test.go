package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/failure"
)

func TestConnectionInputsKeepTransportAndSecretScopesClosed(t *testing.T) {
	authorization := AuthorizationChange{Kind: Set, Value: "Bearer secret"}
	http := ConnectionInput{Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools", Authorization: &authorization}
	if err := http.Validate(); err != nil {
		t.Fatal(err)
	}
	http.Command = "server"
	if err := http.Validate(); err == nil {
		t.Fatal("HTTP connection carrying a stdio command was accepted")
	}
	environment := EnvironmentChange{Kind: Set, Value: map[string]string{"TOKEN": "secret"}}
	stdio := ConnectionInput{Transport: protocol.MCPTransportStdio, Command: "server", Environment: &environment}
	if err := stdio.Validate(); err != nil {
		t.Fatal(err)
	}
	stdio.Authorization = &authorization
	if err := stdio.Validate(); err == nil {
		t.Fatal("stdio connection carrying HTTP authorization was accepted")
	}
	clear := AuthorizationChange{Kind: Clear}
	candidate := Candidate{Name: "docs", Connection: ConnectionInput{Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example", Authorization: &clear}}
	if err := candidate.Validate(); err == nil {
		t.Fatal("candidate clearing a nonexistent secret was accepted")
	}
}

func TestServerAndAuthorizationStatesRejectContradictoryData(t *testing.T) {
	count := 2
	server := Server{
		Name: "docs", Connection: Connection{Transport: protocol.MCPTransportStdio, Command: "docs-server"},
		State: State{Type: Connected, ToolCount: &count},
	}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	server.State.Problem = &failure.Problem{Type: "mcp_dial_failed"}
	if err := server.Validate(); err == nil {
		t.Fatal("connected state carrying a problem was accepted")
	}
	server.State.Problem = nil
	server.DisabledTools = []string{"write"}
	server.AutoApproveTools = []string{"write"}
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted contradictory tool policy")
	}
	now := time.Now()
	attempt := AuthorizationAttempt{ID: "auth_1", Server: "docs", Status: AuthorizationPending, CreatedAt: now}
	if err := attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	attempt.Status = AuthorizationFailed
	if err := attempt.Validate(); err == nil {
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
	authorization := AuthorizationChange{Kind: Set, Value: "Bearer secret"}
	headers := HeadersChange{Kind: Set, Value: map[string]string{"X-Key": "secret"}}
	candidate := Candidate{
		Name: "docs", Enabled: true, Description: "Documentation", HandshakeTimeout: timeout,
		Connection: ConnectionInput{
			Transport: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools",
			Authorization: &authorization, Headers: &headers,
		},
		DisabledTools: []string{"write"}, AutoApproveTools: []string{"search"},
	}
	valid := Server{
		Name: candidate.Name, Description: candidate.Description, HandshakeTimeout: candidate.HandshakeTimeout,
		Connection: Connection{
			Transport: protocol.MCPTransportStreamableHTTP, URL: candidate.Connection.URL,
			AuthorizationMasked: "****", HeadersMasked: map[string]string{"X-Key": "****"},
		},
		DisabledTools: []string{"write"}, AutoApproveTools: []string{"search"},
		State: State{Type: Disconnected},
	}
	if err := candidate.ValidateResult(valid); err != nil {
		t.Fatalf("valid create result: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Server)
		want   string
	}{
		{name: "description", mutate: func(result *Server) { result.Description = "ignored" }, want: "description"},
		{name: "timeout", mutate: func(result *Server) { result.HandshakeTimeout = mustHandshakeTimeout(t, 1) }, want: "timeout"},
		{name: "URL", mutate: func(result *Server) { result.Connection.URL = "https://other.example" }, want: "URL"},
		{name: "authorization", mutate: func(result *Server) { result.Connection.AuthorizationMasked = "" }, want: "authorization"},
		{name: "headers", mutate: func(result *Server) { result.Connection.HeadersMasked = nil }, want: "headers"},
		{name: "enabled", mutate: func(result *Server) { result.State.Type = Disabled }, want: "enabled"},
		{name: "disabled tools", mutate: func(result *Server) { result.DisabledTools = nil }, want: "disabled tools"},
	} {
		t.Run("create "+test.name, func(t *testing.T) {
			result := valid.Clone()
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
	updated := valid.Clone()
	updated.Description, updated.HandshakeTimeout = description, updatedTimeout
	updated.DisabledTools, updated.State = disabledTools, State{Type: Disabled}
	if err := update.ValidateResult(updated); err != nil {
		t.Fatalf("valid update result: %v", err)
	}
	wrongUpdate := updated.Clone()
	wrongUpdate.Description = "ignored"
	if err := update.ValidateResult(wrongUpdate); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("update result error = %v", err)
	}

	clearAuthorization := AuthorizationChange{Kind: Clear}
	clearHeaders := HeadersChange{Kind: Clear}
	connectionUpdate := ServerUpdate{
		Server: candidate.Name,
		Connection: &ConnectionInput{
			Transport: protocol.MCPTransportStreamableHTTP, URL: candidate.Connection.URL,
			Authorization: &clearAuthorization, Headers: &clearHeaders,
		},
	}
	cleared := valid.Clone()
	cleared.Connection.AuthorizationMasked = ""
	cleared.Connection.HeadersMasked = nil
	if err := connectionUpdate.ValidateResult(cleared); err != nil {
		t.Fatalf("valid secret clear result: %v", err)
	}
	uncleared := cleared.Clone()
	uncleared.Connection.AuthorizationMasked = "****"
	if err := connectionUpdate.ValidateResult(uncleared); err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("secret clear result error = %v", err)
	}

	environment := EnvironmentChange{Kind: Set, Value: map[string]string{"TOKEN": "secret"}}
	stdioCandidate := Candidate{
		Name: "local", Enabled: false,
		Connection: ConnectionInput{
			Transport: protocol.MCPTransportStdio, Command: "mcp-server", Args: []string{"--stdio"},
			Environment: &environment, Directory: "/workspace",
		},
	}
	stdioResult := Server{
		Name: stdioCandidate.Name,
		Connection: Connection{
			Transport: protocol.MCPTransportStdio, Command: "mcp-server", Args: []string{"--stdio"},
			EnvironmentMasked: map[string]string{"TOKEN": "****"}, Directory: "/workspace",
		},
		State: State{Type: Disabled},
	}
	if err := stdioCandidate.ValidateResult(stdioResult); err != nil {
		t.Fatalf("valid stdio result: %v", err)
	}
	missingEnvironment := stdioResult.Clone()
	missingEnvironment.Connection.EnvironmentMasked = nil
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
	result := Server{
		Name: candidate.Name, Connection: Connection{Transport: protocol.MCPTransportStdio, Command: "docs-server"},
		DisabledTools: []string{"read", "write"}, AutoApproveTools: []string{"fetch", "search"},
		State: State{Type: Disconnected},
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
