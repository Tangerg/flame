package toolset_test

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	sdkmcp "github.com/Tangerg/go-sdk/mcp"

	scopemcp "github.com/Tangerg/scope/mcp"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// runAsMCPServerEnv is the env-var sentinel that flips this test
// binary into "act as a stdio MCP server" mode. The stdio integration
// test (below) re-execs the test binary with this set so we get a
// real subprocess to talk to over stdin/stdout without depending on
// `npx` / Python / etc. in CI.
const runAsMCPServerEnv = "FLAME_TEST_RUN_AS_MCP_SERVER"

func resolvedRootTools(t *testing.T, resolver *toolset.Resolver) []toolcontract.Tool {
	t.Helper()
	manifest, err := resolver.Manifest(attachedRun(t), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("root manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	return append(append([]toolcontract.Tool(nil), manifest.Visible...), manifest.Deferred...)
}

// TestMain is the standard fork-and-exec trick documented in the
// SDK's cmd_test.go: when FLAME_TEST_RUN_AS_MCP_SERVER is set, run as
// a stdio MCP server (with one `ping` tool) instead of executing
// the test suite.
func TestMain(m *testing.M) {
	if os.Getenv(runAsMCPServerEnv) != "" {
		runStdioMCPServer()
		return
	}
	os.Exit(m.Run())
}

// runStdioMCPServer is the entry point used when the test binary
// re-execs itself as an MCP server. Mirrors the structure of the
// HTTP test server: one `ping` tool, stdio transport.
func runStdioMCPServer() {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name: "flame-test-stdio-mcp", Version: "v0.1.0",
	}, nil)
	ping, err := toolcontract.NewFunc[struct{}, string](
		toolcontract.FuncConfig{
			Name:        "ping",
			Description: "responds with pong",
		},
		func(context.Context, struct{}) (string, error) { return "pong", nil },
	)
	if err != nil {
		log.Fatalf("build tool: %v", err)
	}
	if err := scopemcp.Register(srv, ping); err != nil {
		log.Fatalf("register tools: %v", err)
	}
	transport := &sdkmcp.StdioTransport{}
	if err := srv.Run(context.Background(), transport); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// TestToolEnvironmentDialsMCPServer brings up an in-process MCP server
// over HTTP, registers one tool against it, then constructs an
// tool environment wired to dial the server. Its catalog must
// include the remote tool under its prefixed name; cleanup must
// drop the session cleanly.
func TestToolEnvironmentDialsMCPServer(t *testing.T) {
	// 1. Spin up a real MCP server with one tool.
	mcpServer := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name: "test-srv", Version: "v0.1.0",
	}, nil)
	ping, err := toolcontract.NewFunc[struct{}, string](
		toolcontract.FuncConfig{
			Name:        "ping",
			Description: "responds with pong",
		},
		func(context.Context, struct{}) (string, error) {
			return "pong", nil
		},
	)
	if err != nil {
		t.Fatalf("build tool: %v", err)
	}
	err = scopemcp.Register(mcpServer, ping)
	if err != nil {
		t.Fatalf("register tools: %v", err)
	}

	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return mcpServer },
		nil,
	))
	t.Cleanup(httpServer.Close)

	// 2. Construct the tool environment pointing at the HTTP MCP endpoint.
	built, _ := mustMCPToolEnvironment(t, []mcpserver.Server{{Source: mcpserver.UserSource(), Name: testsupport.ServerName("test"), Transport: mcpserver.TransportStreamableHTTP, URL: httpServer.URL}})

	// 3. The remote tool must appear in the merged list under its
	// model-facing MCP port name.
	want := "test_ping"
	found := false
	for _, tool := range resolvedRootTools(t, built.Resolver) {
		if tool.Definition().Name == want {
			found = true
			break
		}
	}
	if !found {
		catalog := resolvedRootTools(t, built.Resolver)
		names := make([]string, 0, len(catalog))
		for _, t := range catalog {
			names = append(names, t.Definition().Name)
		}
		t.Fatalf("tool %q not in tool catalog; got %v", want, names)
	}
}

// TestToolEnvironmentRejectsDuplicateMCPNames ensures the
// fail-fast guard fires on misconfiguration: two MCPServer
// entries with the same Name must abort tool construction rather than
// silently overwriting.
func TestToolEnvironmentRejectsDuplicateMCPNames(t *testing.T) {
	_, err := mcpconnection.Open(context.Background(), t.Context(), []mcpserver.Server{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("dup"), Transport: mcpserver.TransportStreamableHTTP, URL: "http://example.invalid/"},
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("dup"), Transport: mcpserver.TransportStreamableHTTP, URL: "http://other.invalid/"},
	}, nil, testSourceRegistry(nil))
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

// TestToolEnvironmentRejectsBadMCPEndpoint surfaces validation
// failures at build time so operators don't discover the
// problem on the first tool call.
func TestToolEnvironmentRejectsBadMCPEndpoint(t *testing.T) {
	_, err := mcpconnection.Open(context.Background(), t.Context(), []mcpserver.Server{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("bad"), Transport: mcpserver.TransportStreamableHTTP}, // empty URL fails validation
	}, nil, testSourceRegistry(nil))
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestToolEnvironmentDialsStdioMCP re-execs this test binary as a
// stdio MCP server (see TestMain) and verifies tool construction dials it
// over stdin/stdout, lists its `ping` tool, and surfaces it under
// the `stdio_ping` prefix. Close must terminate the subprocess
// cleanly.
//
// Skipped when the test binary path cannot be resolved (uncommon —
// `go test` always provides argv[0]).
func TestToolEnvironmentDialsStdioMCP(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable not available: %v", err)
	}
	_, err = exec.LookPath(self)
	if err != nil && !fileExists(self) {
		t.Skipf("test binary unreachable for re-exec: %v", err)
	}

	built, _ := mustMCPToolEnvironment(t, []mcpserver.Server{{
		Source:    mcpserver.UserSource(),
		Name:      testsupport.ServerName("stdio"),
		Transport: mcpserver.TransportStdio,
		Command:   self,
		Args:      []string{"-test.run=^$"}, // no test selector — TestMain re-routes
		Env:       map[string]string{runAsMCPServerEnv: "1"},
	}})
	want := "stdio_ping"
	found := false
	for _, tool := range resolvedRootTools(t, built.Resolver) {
		if tool.Definition().Name == want {
			found = true
			break
		}
	}
	if !found {
		catalog := resolvedRootTools(t, built.Resolver)
		names := make([]string, 0, len(catalog))
		for _, t := range catalog {
			names = append(names, t.Definition().Name)
		}
		t.Fatalf("tool %q not in tool catalog; got %v", want, names)
	}
}

// TestToolEnvironmentRejectsEmptyStdioCommand mirrors the
// HTTP empty-endpoint guard for the stdio path.
func TestToolEnvironmentRejectsEmptyStdioCommand(t *testing.T) {
	_, err := mcpconnection.Open(context.Background(), t.Context(), []mcpserver.Server{{
		Source:    mcpserver.UserSource(),
		Name:      testsupport.ServerName("bad"),
		Transport: mcpserver.TransportStdio,
	}}, nil, testSourceRegistry(nil))
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// fileExists is a tiny helper used only by the stdio test's
// re-exec sanity check.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestToolEnvironmentToleratesUnreachableMCP verifies boot tolerance
// (B3b-1): a well-formed but unreachable server is recorded "failed" with its
// reason and skipped, so tool construction still succeeds and serves the rest —
// replacing the old all-or-nothing boot. (A malformed config stays fatal, as
// the sibling Rejects* tests assert.)
func TestToolEnvironmentToleratesUnreachableMCP(t *testing.T) {
	built, pool := mustMCPToolEnvironment(t, []mcpserver.Server{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("down"), Transport: mcpserver.TransportStreamableHTTP, URL: "http://127.0.0.1:1/mcp"},
	})
	statuses := pool.Statuses()
	if len(statuses) != 1 || statuses[0].Server != testsupport.UserMCPServer("down") || statuses[0].State != mcpserver.ConnectionFailed {
		t.Fatalf("statuses = %+v, want [down failed]", statuses)
	}
	tools, _, err := built.Resolver.MCPTools(nil)
	if err != nil {
		t.Fatalf("MCPTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("MCPTools = %+v, want empty (no connected server)", tools)
	}
}

// TestToolEnvironmentReconnectsMCP covers the reconnect path against an
// unreachable server: the dial still fails, so the server walks connecting →
// failed (returning the error) and its tools stay absent; an unknown name is
// mcpserver.ErrUnknownServer. (A successful reconnect's tool hot-swap rides the same
// code path as boot, which the stdio integration test already exercises.)
func TestToolEnvironmentReconnectsMCP(t *testing.T) {
	built, pool := mustMCPToolEnvironment(t, []mcpserver.Server{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("down"), Transport: mcpserver.TransportStreamableHTTP, URL: "http://127.0.0.1:1/mcp"},
	})
	if err := pool.Reconnect(context.Background(), testsupport.UserMCPServer("down")); err == nil {
		t.Fatal("reconnect of an unreachable server must return the dial error")
	}
	st := pool.Statuses()
	if len(st) != 1 || st[0].State != mcpserver.ConnectionFailed {
		t.Fatalf("statuses = %+v, want [down failed]", st)
	}
	if tools, _, _ := built.Resolver.MCPTools(nil); len(tools) != 0 {
		t.Fatalf("MCPTools = %+v, want empty after a failed reconnect", tools)
	}

	if err := pool.Reconnect(context.Background(), testsupport.UserMCPServer("ghost")); !errors.Is(err, mcpserver.ErrUnknownServer) {
		t.Fatalf("reconnect unknown = %v, want mcpserver.ErrUnknownServer", err)
	}
}

func mustMCPToolEnvironment(t *testing.T, servers []mcpserver.Server) (toolset.Built, *mcpconnection.Pool) {
	t.Helper()
	pool, err := mcpconnection.Open(t.Context(), t.Context(), servers, nil, testSourceRegistry(servers))
	if err != nil {
		t.Fatalf("Open MCP pool: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{Lifetime: t.Context(),
		UserHome: t.TempDir(),
	})
	if err != nil {
		_ = pool.Shutdown(context.WithoutCancel(t.Context()))
		t.Fatalf("Build toolset: %v", err)
	}
	pool.SetToolSink(built.Resolver.SetMCPTools)
	t.Cleanup(func() {
		for index := len(built.Closers) - 1; index >= 0; index-- {
			if closeFn := built.Closers[index]; closeFn != nil {
				_ = closeFn()
			}
		}
		_ = pool.Shutdown(context.WithoutCancel(t.Context()))
	})
	return built, pool
}

func TestMCPNameCollisionsAfterDialAndReconnect(t *testing.T) {
	serve := func(name string, tools ...string) (*sdkmcp.Server, string) {
		t.Helper()
		remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: name, Version: "v1"}, nil)
		for _, name := range tools {
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: name}, func(context.Context, struct{}) (string, error) { return "ok", nil })
			if err != nil {
				t.Fatal(err)
			}
			if err := scopemcp.Register(remote, executable); err != nil {
				t.Fatal(err)
			}
		}
		server := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
		t.Cleanup(server.Close)
		return remote, server.URL
	}
	_, firstURL := serve("first", "c", "healthy")
	second, secondURL := serve("second", "b_c", "healthy")
	for _, reversed := range []bool{false, true} {
		servers := []mcpserver.Server{
			{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a_b"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: firstURL},
			{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: secondURL},
		}
		if reversed {
			servers[0], servers[1] = servers[1], servers[0]
		}
		built, pool := mustMCPToolEnvironment(t, servers)
		verify := func(collision bool) {
			t.Helper()
			for _, status := range pool.Statuses() {
				if status.State != mcpserver.ConnectionConnected {
					t.Fatalf("server disconnected: %+v", status)
				}
			}
			catalog := resolvedRootTools(t, built.Resolver)
			names := make(map[string]bool)
			for _, executable := range catalog {
				names[executable.Definition().Name] = true
			}
			if names["a_b_c"] == collision || !names["a_b_healthy"] || !names["a_healthy"] {
				t.Fatalf("manifest tools = %v", names)
			}
			if _, err := toolcontract.NewRegistry(catalog...); err != nil {
				t.Fatal(err)
			}
			_, conflicts, err := built.Resolver.MCPTools(nil)
			if err != nil {
				t.Fatal(err)
			}
			if collision && len(conflicts) != 2 || !collision && len(conflicts) != 0 {
				t.Fatalf("conflicts = %v", conflicts)
			}
		}
		verify(true)
		if err := pool.Reconnect(t.Context(), testsupport.UserMCPServer("a")); err != nil {
			t.Fatal(err)
		}
		verify(true)
		if reversed {
			second.RemoveTools("b_c")
			if err := pool.Reconnect(t.Context(), testsupport.UserMCPServer("a")); err != nil {
				t.Fatal(err)
			}
			verify(false)
		}
	}
}

type testSourceRegistry []mcpserver.Server

func (r testSourceRegistry) Dispatchable(_ context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	for _, server := range r {
		if server.ID() == id {
			server.Enabled = true
			return server, true, nil
		}
	}
	return mcpserver.Server{}, false, nil
}

func (r testSourceRegistry) Connection(ctx context.Context, name mcpserver.ID) (mcpapp.Launch, error) {
	server, found, err := r.Dispatchable(ctx, name)
	if err == nil && !found {
		err = mcpserver.ErrUnknownServer
	}
	return mcpapp.Launch{Server: server}, err
}

// attachedRun is the Run scope every Tool resolution executes under.
func attachedRun(t *testing.T) context.Context {
	t.Helper()
	workspace := t.TempDir()
	return executionctx.WithScope(t.Context(), runs.ExecutionScope{
		SessionID: "session-test", CWD: workspace, WorkspaceCWD: workspace,
	})
}
