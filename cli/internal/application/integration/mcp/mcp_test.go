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

// TestAuthorizationAttemptRejectsReversedTimestamps covers the one fact the
// Runtime wire contract cannot state about an attempt. Field presence, closed
// status values, and identity syntax are the endpoint's answer, given with the
// same generated validators before the value reached the CLI; restating them
// here would be a second implementation that is free to drift.
func TestAuthorizationAttemptRejectsReversedTimestamps(t *testing.T) {
	now := time.Now()
	attempt := protocol.MCPAuthorizationAttempt{
		ID: "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA", Server: UserServer("docs"),
		Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending}, CreatedAt: now,
	}
	if err := ValidateAuthorizationAttempt(attempt); err != nil {
		t.Fatal(err)
	}
	finished := now.Add(-time.Second)
	attempt.FinishedAt = &finished
	if err := ValidateAuthorizationAttempt(attempt); err == nil {
		t.Fatal("authorization finishing before it started was accepted")
	}
}

func TestServerUpdateRequiresAnExplicitChange(t *testing.T) {
	if err := (ServerUpdate{Server: UserServer("docs")}).Validate(); err == nil {
		t.Fatal("empty MCP update was accepted")
	}
	description := "Documentation tools"
	if err := (ServerUpdate{Server: UserServer("docs"), Description: &description}).Validate(); err != nil {
		t.Fatal(err)
	}
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

func TestServerReferenceReadsBothOriginsAndRejectsMalformedText(t *testing.T) {
	installed := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: "eeb329cd-c7ce-40c9-bd90-6821fef06d30"}, Name: "files"}
	for text, want := range map[string]protocol.MCPServerID{
		"files":   UserServer("files"),
		" files ": UserServer("files"),
		"eeb329cd-c7ce-40c9-bd90-6821fef06d30/files": installed,
	} {
		got, err := ParseServerReference(text)
		if err != nil || got != want {
			t.Errorf("ParseServerReference(%q) = %+v, %v", text, got, err)
		}
		if parsed, err := ParseServerReference(ServerLabel(want)); err != nil || parsed != want {
			t.Errorf("label of %+v does not read back: %+v, %v", want, parsed, err)
		}
	}
	for _, text := range []string{"", "Files", "installation/eeb329cd-c7ce-40c9-bd90-6821fef06d30/files", "not-a-uuid/files", "eeb329cd-c7ce-40c9-bd90-6821fef06d30/"} {
		if _, err := ParseServerReference(text); err == nil {
			t.Errorf("ParseServerReference(%q) accepted malformed text", text)
		}
	}
}
