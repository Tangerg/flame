package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type mcpServerListStub struct {
	servers  []mcpserver.Server
	disabled []tool.Ref
	err      error
	calls    int
}

func (m *mcpServerListStub) List(context.Context) ([]mcpserver.Server, error) {
	m.calls++
	return m.servers, m.err
}

func TestBuildMCPEnvironmentUsesOneRegistrySnapshot(t *testing.T) {
	registry := &mcpServerListStub{servers: []mcpserver.Server{
		{Name: testMCPServerName("files"), Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"},
		{Name: testMCPServerName("off"), Enabled: false, Transport: mcpserver.TransportStdio, Command: "mcp-off"},
	}, disabled: []tool.Ref{testMCPRef("files", "write")}}

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
	if !env.exposure.ToolDisabled(testMCPRef("files", "write")) ||
		!env.exposure.ToolDisabled(testMCPRef("off", "hidden")) {
		t.Fatalf("disabled policy does not match registry snapshot")
	}
	if env.exposure.ToolDisabled(testMCPRef("files", "read")) {
		t.Fatal("files_read must remain exposed")
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

func (m *mcpServerListStub) ListExposure(context.Context) ([]tool.Ref, error) {
	return m.disabled, m.err
}
func testMCPRef(server, remote string) tool.Ref {
	ref, err := tool.MCP(testMCPServerName(server), testRemoteToolName(remote))
	if err != nil {
		panic(err)
	}
	return ref
}
