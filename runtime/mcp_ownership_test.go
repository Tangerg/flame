package runtime

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRuntimeStartupDoesNotCreateMCPServersFromEnvironment(t *testing.T) {
	config := isolatedMCPRuntimeConfig(t)
	var requests atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer endpoint.Close()
	t.Setenv("FLAME_MCP_SERVERS", "managed="+endpoint.URL)
	t.Setenv("FLAME_MCP_MANAGED_TOKEN", "environment-credential")

	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	servers, err := runtime.ListMCPServers(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers.Data) != 0 || requests.Load() != 0 {
		t.Fatalf("environment created %d servers and %d connection requests", len(servers.Data), requests.Load())
	}
}

func TestRuntimeRestartPreservesMCPResourceChanges(t *testing.T) {
	config := isolatedMCPRuntimeConfig(t)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer endpoint.Close()
	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	created, err := runtime.CreateMCPServer(t.Context(), protocol.MCPServerCandidate{
		Name:        "managed",
		Enabled:     false,
		Description: "resource-owned configuration",
		Connection: protocol.MCPConnectionInput{
			Type: protocol.MCPTransportStreamableHTTP,
			URL:  endpoint.URL + "/stored",
			Authorization: &protocol.MCPAuthorizationChange{
				Type: protocol.MCPSecretSet, Value: "Bearer resource-credential",
			},
		},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, CommandOptions{IdempotencyKey: "create-managed-server"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_MCP_SERVERS", "managed="+endpoint.URL+"/environment")
	t.Setenv("FLAME_MCP_MANAGED_TOKEN", "environment-credential")
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	servers, err := reopened.ListMCPServers(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers.Data) != 1 {
		t.Fatalf("servers after restart = %+v", servers.Data)
	}
	stored := servers.Data[0]
	if stored.ID != created.ID || stored.Status.Type != protocol.MCPServerDisabled ||
		stored.Description != created.Description || stored.Connection.URL != created.Connection.URL ||
		stored.Connection.AuthorizationMasked != created.Connection.AuthorizationMasked {
		t.Fatalf("restart changed resource configuration: got %+v, want %+v", stored, created)
	}
	if err := reopened.DeleteMCPServer(t.Context(), protocol.MCPServerRequest{Server: created.ID}, CommandOptions{
		IdempotencyKey: "delete-managed-server",
	}); err != nil {
		t.Fatal(err)
	}
	servers, err = reopened.ListMCPServers(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers.Data) != 0 {
		t.Fatalf("servers after deletion = %+v", servers.Data)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	afterDeletion, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = afterDeletion.Close() })
	servers, err = afterDeletion.ListMCPServers(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers.Data) != 0 {
		t.Fatalf("restart resurrected deleted MCP resources: %+v", servers.Data)
	}
}

func isolatedMCPRuntimeConfig(t *testing.T) Config {
	t.Helper()
	t.Setenv("FLAME_PROVIDER", "anthropic")
	for _, name := range []string{
		"FLAME_MODEL", "FLAME_APIKEY", "FLAME_BASEURL", "ANTHROPIC_API_KEY",
		"FLAME_MCP_SERVERS", "FLAME_A2A_AGENTS", "FLAME_A2A_RPC_ORIGINS",
	} {
		t.Setenv(name, "")
	}
	return Config{
		DataDirectory:        t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		UserHomePath:         t.TempDir(),
		ConfigDirectories:    []string{t.TempDir()},
	}
}
