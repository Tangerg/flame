package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type mcpServerListStub struct {
	servers []mcpserver.Server
	err     error
	calls   int
}

func (m *mcpServerListStub) List(context.Context) ([]mcpserver.Server, error) {
	m.calls++
	return m.servers, m.err
}

func TestBuildMCPEnvironmentUsesOneRegistrySnapshot(t *testing.T) {
	registry := &mcpServerListStub{servers: []mcpserver.Server{
		{Name: testMCPServerName("files"), Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files", ToolPolicy: testServerToolPolicy([]string{"write"}, []string{"read"})},
		{Name: testMCPServerName("off"), Enabled: false, Transport: mcpserver.TransportStdio, Command: "mcp-off", ToolPolicy: testServerToolPolicy([]string{"hidden"}, nil)},
	}}

	env, err := buildMCPEnvironment(context.Background(), registry)
	if err != nil {
		t.Fatalf("buildMCPEnvironment: %v", err)
	}
	if registry.calls != 1 {
		t.Fatalf("registry List calls = %d, want 1", registry.calls)
	}
	if len(env.servers) != 1 || env.servers[0].Name.String() != "files" {
		t.Fatalf("servers = %+v, want enabled files server", env.servers)
	}
	if !env.policy.ToolDisabled(mcpserver.ToolRef{Server: testMCPServerName("files"), Tool: testRemoteToolName("write")}) ||
		!env.policy.ToolDisabled(mcpserver.ToolRef{Server: testMCPServerName("off"), Tool: testRemoteToolName("hidden")}) {
		t.Fatalf("disabled policy does not match registry snapshot")
	}
	if !env.policy.ToolAutoApproved(mcpserver.ToolRef{Server: testMCPServerName("files"), Tool: testRemoteToolName("read")}) {
		t.Fatal("files_read must be auto-approved")
	}
}

func TestBuildMCPEnvironmentReturnsRegistryError(t *testing.T) {
	want := errors.New("registry unavailable")
	registry := &mcpServerListStub{err: want}

	_, err := buildMCPEnvironment(context.Background(), registry)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if registry.calls != 1 {
		t.Fatalf("registry List calls = %d, want 1", registry.calls)
	}
}
