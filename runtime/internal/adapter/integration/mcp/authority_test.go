package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestBorrowBoundSessionCannotBorrowChangedOrWithdrawnAuthority(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "original-owner"}, nil)
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	defer transport.Close()
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("schedules"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Shutdown(context.WithoutCancel(t.Context()))
	original, err := pool.BorrowBoundSession(t.Context(), server.ID(), server.AuthorityFingerprint())
	if err != nil || original == nil {
		t.Fatalf("original session: %v", err)
	}
	defer original.Close()
	for _, change := range []struct {
		name  string
		apply func(*mcpserver.Server)
	}{
		{"disabled", func(s *mcpserver.Server) { s.Enabled = false }},
		{"other endpoint", func(s *mcpserver.Server) { s.URL += "/replacement" }},
		{"other source", func(s *mcpserver.Server) { s.Name = testsupport.ServerName("replacement") }},
	} {
		t.Run(change.name, func(t *testing.T) {
			current := server.Clone()
			change.apply(&current)
			registry.set(current)
			if session, err := pool.BorrowBoundSession(t.Context(), server.ID(), server.AuthorityFingerprint()); session != nil || !errors.Is(err, ErrUnavailable) {
				t.Fatalf("replacement answered as original authority: %v", err)
			}
		})
	}
}

func TestPreparedToolObservesUncertainReplyOnlyAfterAdmittedDispatch(t *testing.T) {
	var calls atomic.Int32
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "schedule-owner"}, nil)
	remote.AddTool(&sdkmcp.Tool{Name: "write", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		return nil, errors.New("reply failed after dispatch")
	})
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("schedules"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	var settlements atomic.Int32
	pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, func(ctx context.Context, source Executable, _ ToolHost) (ToolCall, func(), error) {
		return func(invocation toolcontract.Invocation) (toolcontract.Result, error) {
			defer settlements.Add(1)
			return source.Call(ctx, invocation)
		}, func() {}, nil
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	var tools []toolcontract.Tool
	awaitPoolStartup(t, pool)
	pool.SetToolSink(func(current []toolcontract.Tool, _ []mcpserver.ID) { tools = current })
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want one admitted remote tool", len(tools))
	}
	settlements.Store(0)
	binding, err := toolcontract.Bind(tools[0])
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "write", Name: binding.Contract().Definition().Name, Arguments: `{}`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := binding.Call(operation.Start(t.Context(), extension.Model, "test.model", "read"), invocation); err == nil {
		t.Fatal("failed reply appeared successful")
	}
	if calls.Load() != 1 || settlements.Load() != 1 {
		t.Fatalf("dispatch=%d settlements=%d, want one observation after uncertain dispatch", calls.Load(), settlements.Load())
	}
	server.Enabled = false
	registry.set(server)
	if _, err := binding.Call(operation.Start(t.Context(), extension.Model, "test.model", "read"), invocation); err == nil {
		t.Fatal("withdrawn source dispatched")
	}
	if calls.Load() != 1 || settlements.Load() != 1 {
		t.Fatal("withdrawn source produced remote work or a change notice")
	}
}

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
			server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL, Authorization: "Bearer first"}
			if credential == "header" {
				server.Headers = map[string]string{"X-API-Key": "first"}
			}
			registry := &mutableSourceRegistry{server: server.Clone()}
			pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			var initial []toolcontract.Tool
			awaitPoolStartup(t, pool)
			pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { initial = tools })
			if len(initial) != 1 {
				t.Fatalf("initial tools = %d, want 1", len(initial))
			}
			retained, err := toolcontract.Bind(initial[0])
			if err != nil {
				t.Fatal(err)
			}
			invoke := func(binding toolcontract.Binding) error {
				invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: binding.Contract().Definition().Name, Arguments: `{}`}, nil)
				if err != nil {
					return err
				}
				_, err = binding.Call(operation.Start(t.Context(), extension.Model, "test.model", "read"), invocation)
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
			awaitPoolStartup(t, pool)
			pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { replacement = tools })
			if err := connectPool(pool, t.Context(), server.ID()); err != nil {
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

func (r *mutableSourceRegistry) Definition(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.server.Clone(), true, nil
}

func (r *mutableSourceRegistry) Dispatchable(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.Definition(ctx, id)
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
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	var initial []toolcontract.Tool
	awaitPoolStartup(t, pool)
	pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { initial = tools })
	if len(initial) != 1 {
		t.Fatalf("initial tools = %d, want 1", len(initial))
	}
	invoke := func(executable toolcontract.Tool) error {
		binding, err := toolcontract.Bind(executable)
		if err != nil {
			return err
		}
		invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: binding.Contract().Definition().Name, Arguments: `{}`}, nil)
		if err != nil {
			return err
		}
		_, err = binding.Call(operation.Start(t.Context(), extension.Model, "test.model", "read"), invocation)
		return err
	}
	var replacement []toolcontract.Tool
	awaitPoolStartup(t, pool)
	pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { replacement = tools })
	if err := pool.Refuse(t.Context(), server.ID(), mcpserver.FailureConfiguration); err != nil {
		t.Fatal(err)
	}
	if err := connectPool(pool, t.Context(), server.ID()); err != nil {
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
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: oldRemote.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	server.URL = currentRemote.URL
	registry.set(server)
	var replacement []toolcontract.Tool
	awaitPoolStartup(t, pool)
	pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { replacement = tools })
	if err := connectPool(pool, t.Context(), server.ID()); err != nil {
		t.Fatal(err)
	}
	if len(replacement) != 1 {
		t.Fatalf("current catalog = %d tools", len(replacement))
	}
	binding, err := toolcontract.Bind(replacement[0])
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: binding.Contract().Definition().Name, Arguments: `{}`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := binding.Call(operation.Start(t.Context(), extension.Model, "test.model", "read"), invocation); err != nil {
		t.Fatalf("reconnect published superseded executable: %v", err)
	}
	if oldCalls.Load() != 0 || currentCalls.Load() != 1 {
		t.Fatalf("dispatch old=%d current=%d", oldCalls.Load(), currentCalls.Load())
	}
}

func (r *mutableSourceRegistry) Connection(ctx context.Context, name mcpserver.ID) (mcpapp.Launch, error) {
	r.mu.Lock()
	refusal := r.refusal
	r.mu.Unlock()
	if refusal != nil {
		return mcpapp.Launch{}, refusal
	}
	server, _, err := r.Dispatchable(ctx, name)
	return mcpapp.Launch{Server: server}, err
}

func TestAuthorizationUsesCurrentOwnerCredentials(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "oauth-owner"}, nil)
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
	registry := &mutableSourceRegistry{server: server.Clone()}
	pool, err := Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, registry, nil, nil, nil, nil, nil)
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
	if err := authorizePool(pool, t.Context(), server.ID()); err == nil || !strings.Contains(err.Error(), "static authorization") {
		t.Fatalf("authorization ignored the owner's static credentials: %v", err)
	}
}
