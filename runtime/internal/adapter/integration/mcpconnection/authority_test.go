package mcpconnection

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestRetainedToolRejectsChangedCredentialsBeforeReconciliation(t *testing.T) {
	for _, credential := range []string{"authorization", "header"} {
		t.Run(credential, func(t *testing.T) {
			var calls atomic.Int32
			var observedCredential atomic.Value
			remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "credential-owner"}, nil)
			remote.AddTool(&sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				calls.Add(1)
				return &sdkmcp.CallToolResult{}, nil
			})
			handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil)
			transport := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					key := "Authorization"
					if credential == "header" {
						key = "X-API-Key"
					}
					observedCredential.Store(request.Header.Get(key))
				}
				handler.ServeHTTP(response, request)
			}))
			t.Cleanup(transport.Close)
			server := mcpserver.Server{Source: testInstallationSource(t), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL, Authorization: "Bearer first"}
			if credential == "header" {
				server.Headers = map[string]string{"X-API-Key": "first"}
			}
			registry := &mutableSourceRegistry{server: server.Clone()}
			pool, initial, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			if len(initial) != 1 {
				t.Fatalf("initial tools = %d, want 1", len(initial))
			}
			retained, err := toolcontract.Bind(initial[0])
			if err != nil {
				t.Fatal(err)
			}
			invoke := func(binding toolcontract.Binding) error {
				invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: binding.Contract().Definition().Name, Arguments: `{}`})
				if err != nil {
					return err
				}
				_, err = binding.Call(t.Context(), invocation)
				return err
			}
			if err := invoke(retained); err != nil {
				t.Fatal(err)
			}
			if credential == "authorization" {
				server.Authorization = "Bearer second"
			} else {
				server.Headers["X-API-Key"] = "second"
			}
			if server.AuthorityFingerprint() != registry.server.AuthorityFingerprint() {
				t.Fatal("credential rotation changed permission authority")
			}
			registry.set(server)
			if err := invoke(retained); err == nil {
				t.Fatal("retained executable dispatched superseded credentials before reconciliation")
			} else {
				var failure *toolcontract.Failure
				if !errors.As(err, &failure) || failure.Kind() != toolcontract.FailureKindRejected {
					t.Fatalf("superseded configuration outcome = %v, want rejected", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatal("rejected invocation reached the remote server")
			}
			var replacement []toolcontract.Tool
			pool.SetToolSink(func(tools []toolcontract.Tool) { replacement = tools })
			if err := pool.Configure(t.Context(), server.ID()); err != nil {
				t.Fatal(err)
			}
			if len(replacement) != 1 {
				t.Fatalf("replacement tools = %d, want 1", len(replacement))
			}
			current, err := toolcontract.Bind(replacement[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := invoke(current); err != nil {
				t.Fatalf("current executable: %v", err)
			}
			if calls.Load() != 2 {
				t.Fatalf("remote calls = %d, want 2", calls.Load())
			}
			want := server.Authorization
			if credential == "header" {
				want = server.Headers["X-API-Key"]
			}
			if observedCredential.Load() != want {
				t.Fatal("current executable did not use current credentials")
			}
		})
	}
}

type mutableSourceRegistry struct {
	mu      sync.Mutex
	server  mcpserver.Server
	refusal error
}

func (r *mutableSourceRegistry) refuse(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refusal = err
}

func (r *mutableSourceRegistry) Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.server.Clone(), true, nil
}

func (r *mutableSourceRegistry) set(server mcpserver.Server) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.server = server.Clone()
}

// A Run's frozen executable outlives a refusal and a later reconnect with the
// same configuration. Its session is closed by then, so dispatch rechecks the
// connection like any revocable authority and refuses definitely instead of
// surfacing a transport failure from the retired session.
func TestFrozenExecutableAfterReconnectIsRejected(t *testing.T) {
	var calls atomic.Int32
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "connection-owner"}, nil)
	remote.AddTool(&sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		return &sdkmcp.CallToolResult{}, nil
	})
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	server := mcpserver.Server{Source: testInstallationSource(t), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, initial, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	if len(initial) != 1 {
		t.Fatalf("initial tools = %d, want 1", len(initial))
	}
	invoke := func(executable toolcontract.Tool) error {
		binding, err := toolcontract.Bind(executable)
		if err != nil {
			return err
		}
		invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: binding.Contract().Definition().Name, Arguments: `{}`})
		if err != nil {
			return err
		}
		_, err = binding.Call(t.Context(), invocation)
		return err
	}
	var replacement []toolcontract.Tool
	pool.SetToolSink(func(tools []toolcontract.Tool) { replacement = tools })
	if err := pool.Refuse(t.Context(), server.ID(), mcpserver.FailureConfiguration); err != nil {
		t.Fatal(err)
	}
	if err := pool.Reconnect(t.Context(), server.ID()); err != nil {
		t.Fatal(err)
	}
	if len(replacement) != 1 {
		t.Fatalf("replacement tools = %d, want 1", len(replacement))
	}
	err = invoke(initial[0])
	var failure *toolcontract.Failure
	if !errors.As(err, &failure) || failure.Kind() != toolcontract.FailureKindRejected {
		t.Fatalf("frozen executable outcome = %v, want a definite rejection", err)
	}
	if text, _ := failure.Output().Text(); !strings.Contains(text, "source connection is no longer current") {
		t.Fatalf("rejection output = %q", text)
	}
	if calls.Load() != 0 {
		t.Fatal("frozen executable reached the remote server")
	}
	if err := invoke(replacement[0]); err != nil || calls.Load() != 1 {
		t.Fatalf("current executable = %v, calls = %d", err, calls.Load())
	}
}

func TestReconnectUsesCurrentOwnerConfiguration(t *testing.T) {
	openRemote := func(calls *atomic.Int32) *httptest.Server {
		remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "current-owner"}, nil)
		remote.AddTool(&sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			calls.Add(1)
			return &sdkmcp.CallToolResult{}, nil
		})
		transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
		t.Cleanup(transport.Close)
		return transport
	}
	var oldCalls, currentCalls atomic.Int32
	oldRemote, currentRemote := openRemote(&oldCalls), openRemote(&currentCalls)
	server := mcpserver.Server{Source: testInstallationSource(t), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: oldRemote.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, _, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	server.URL = currentRemote.URL
	registry.set(server)
	var replacement []toolcontract.Tool
	pool.SetToolSink(func(tools []toolcontract.Tool) { replacement = tools })
	if err := pool.Reconnect(t.Context(), server.ID()); err != nil {
		t.Fatal(err)
	}
	if len(replacement) != 1 {
		t.Fatalf("current catalog = %d tools", len(replacement))
	}
	binding, err := toolcontract.Bind(replacement[0])
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: binding.Contract().Definition().Name, Arguments: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := binding.Call(t.Context(), invocation); err != nil {
		t.Fatalf("reconnect published superseded executable: %v", err)
	}
	if oldCalls.Load() != 0 || currentCalls.Load() != 1 {
		t.Fatalf("dispatch old=%d current=%d", oldCalls.Load(), currentCalls.Load())
	}
}

func (r *mutableSourceRegistry) Connection(ctx context.Context, name mcpserver.ID) (mcpserver.Server, error) {
	r.mu.Lock()
	refusal := r.refusal
	r.mu.Unlock()
	if refusal != nil {
		return mcpserver.Server{}, refusal
	}
	server, _, err := r.Dispatchable(ctx, name)
	return server, err
}

func TestAuthorizationUsesCurrentOwnerCredentials(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "oauth-owner"}, nil)
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, _, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	server.Authorization = "Bearer local-test"
	registry.set(server)
	// Cancellation prevents a regressed OAuth path from opening the system browser.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := pool.Authorize(ctx, server.ID()); err == nil || !strings.Contains(err.Error(), "static authorization") {
		t.Fatalf("authorization ignored the owner's static credentials: %v", err)
	}
}
