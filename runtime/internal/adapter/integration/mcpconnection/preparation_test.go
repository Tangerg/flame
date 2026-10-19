package mcpconnection

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	inframcp "github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type preparationSourceRegistry struct {
	*mutableSourceRegistry
	unreadable atomic.Bool
}

func (r *preparationSourceRegistry) Dispatchable(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
	if r.unreadable.Load() {
		return mcpserver.Server{}, false, errors.New("source storage unavailable")
	}
	return r.mutableSourceRegistry.Dispatchable(ctx, id)
}

type canceledSourceRegistry struct {
	*mutableSourceRegistry
	cancel     context.CancelCauseFunc
	readError  error
	withdrawal error
}

func (r canceledSourceRegistry) Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	r.cancel(r.withdrawal)
	return mcpserver.Server{}, false, r.readError
}

func TestSourceReadFailureAndConcurrentCancellationRemainObservable(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	readError := errors.New("source storage read failed")
	withdrawal := errors.New("call owner withdrew admission")
	tool := authorizedTool{registry: canceledSourceRegistry{cancel: cancel, readError: readError, withdrawal: withdrawal}}
	if _, err := tool.Call(ctx, toolcontract.Invocation{}); !errors.Is(err, readError) || !errors.Is(err, context.Canceled) || !errors.Is(err, withdrawal) {
		t.Fatalf("source read or lifetime cause was lost: %v", err)
	}
}

func TestPreparedToolRechecksAuthorityAndReleasesBeforeRefusedDispatch(t *testing.T) {
	for _, change := range []string{"disabled", "configuration", "detached", "canceled", "source read failure", "invalid prepared call"} {
		t.Run(change, func(t *testing.T) {
			var calls, releases, settled atomic.Int32
			remote := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
			remote.AddTool(&sdk.Tool{Name: "write", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				calls.Add(1)
				return &sdk.CallToolResult{}, nil
			})
			transport := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
			t.Cleanup(transport.Close)
			server := mcpserver.Server{Source: testInstallationSource(t), Name: testsupport.ServerName("owner"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
			registry := &preparationSourceRegistry{mutableSourceRegistry: &mutableSourceRegistry{server: server.Clone()}}
			prepared := make(chan struct{})
			resume := make(chan struct{})
			finish := sync.OnceFunc(func() { close(resume) })
			t.Cleanup(finish)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			pool, err := Open(ctx, ctx, []mcpserver.Server{server}, nil, registry, nil, func(ctx context.Context, source inframcp.Executable, _ ToolHost) (ToolCall, func(), error) {
				close(prepared)
				select {
				case <-resume:
				case <-ctx.Done():
				}
				if change == "invalid prepared call" {
					return nil, func() { releases.Add(1) }, nil
				}
				return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
					defer settled.Add(1)
					return source.Call(ctx, invocation)
				}, func() { releases.Add(1) }, nil
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				finish()
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			awaitPoolStartup(t, pool)
			var tools []toolcontract.Tool
			pool.SetToolSink(func(current []toolcontract.Tool, _ []mcpserver.ID) { tools = current })
			if len(tools) != 1 {
				t.Fatal("fixture did not publish its remote tool")
			}
			bound, err := toolcontract.Bind(tools[0])
			if err != nil {
				t.Fatal(err)
			}
			invocation, err := bound.Contract().Prepare(chat.ToolCall{ID: "write", Name: bound.Contract().Definition().Name, Arguments: `{}`})
			if err != nil {
				t.Fatal(err)
			}
			callCtx, cancelCall := context.WithCancelCause(operation.Start(ctx, capability.Model, "test.model", "write"))
			defer cancelCall(nil)
			result := make(chan error, 1)
			go func() { _, err := bound.Call(callCtx, invocation); result <- err }()
			select {
			case <-prepared:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			cause := errors.New("preparation withdrawn")
			switch change {
			case "disabled":
				server.Enabled = false
				registry.set(server)
			case "configuration":
				server.Headers = map[string]string{"x-source": "replacement"}
				registry.set(server)
			case "detached":
				if err := pool.Detach(server.ID()); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				cancelCall(cause)
			case "source read failure":
				registry.unreadable.Store(true)
			}
			finish()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("prepared call bypassed final admission")
				}
				if change == "canceled" && (!errors.Is(err, context.Canceled) || !errors.Is(err, cause)) {
					t.Fatalf("cancellation lost its original cause: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if calls.Load() != 0 || releases.Load() != 1 || settled.Load() != 0 {
				t.Fatalf("refusal performed remote work or leaked preparation: calls=%d releases=%d settlements=%d", calls.Load(), releases.Load(), settled.Load())
			}
		})
	}
}
