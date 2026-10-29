package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type toolInstallationStore struct {
	plugins.Store
	snapshot plugin.Snapshot
	host     *qimu.Host
	failure  error
}

func (s *toolInstallationStore) Get(ctx context.Context, id identity.InstallationID) (plugin.Snapshot, bool, error) {
	if s.failure != nil {
		return plugin.Snapshot{}, false, s.failure
	}
	if s.Store != nil {
		return s.Store.Get(ctx, id)
	}
	return s.snapshot, true, nil
}

func (s *toolInstallationStore) List(ctx context.Context) ([]plugin.Snapshot, error) {
	if s.failure != nil {
		return nil, s.failure
	}
	if s.Store != nil {
		return s.Store.List(ctx)
	}
	return []plugin.Snapshot{s.snapshot}, nil
}

type toolSourceRegistry struct{ server mcpserver.Server }

func (r toolSourceRegistry) Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.server.Clone(), true, nil
}
func (r toolSourceRegistry) Definition(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return r.server.Clone(), true, nil
}
func (r toolSourceRegistry) Connection(context.Context, mcpserver.ID) (mcpapp.Launch, error) {
	return mcpapp.Launch{Server: r.server.Clone()}, nil
}

type preparedEmbedder struct{ calls atomic.Int32 }

func (*preparedEmbedder) ID() string { return "prepared-space" }
func (e *preparedEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	e.calls.Add(1)
	return [][]float32{{1, 0}}, nil
}

func TestFeatureToolPreparationKeepsSourceMetadataAndCallLifetime(t *testing.T) {
	memoryContracts, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	schedulesContracts, err := portable.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		contract  *portable.Capability
		name      string
		embedding bool
	}{{memoryContracts, memorycontract.SearchToolName, true}, {memoryContracts, "add_memory", false}, {schedulesContracts, "search", false}} {
		contract := scenario.contract
		role := contract.Descriptor().Reference
		t.Run(role+"/"+scenario.name, func(t *testing.T) {
			name := testsupport.ServerName("owner")
			release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.tools", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "echo"}}, Contributions: []plugin.Contribution{{ID: "tool-provider", Server: name, Export: "owner", Contract: contract.Identity()}}})
			if err != nil {
				t.Fatal(err)
			}
			installation, err := plugin.New(testsupport.InstallationID(t), "test source", release)
			if err != nil {
				t.Fatal(err)
			}
			if err := installation.Enable(release); err != nil {
				t.Fatal(err)
			}
			source, err := installation.ServerSource(release, name)
			if err != nil {
				t.Fatal(err)
			}
			store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
			callbacks := qimumcp.NewCallbacks()
			t.Cleanup(func() { _ = callbacks.Close() })
			if err := callbacks.PublishRPC(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unexpected host call", http.StatusBadRequest)
			})); err != nil {
				t.Fatal(err)
			}
			var calls, notices atomic.Int32
			embedder := &preparedEmbedder{}
			var captured pluginsdk.ToolContext
			var capturedGrant qimumcp.CallbackRef
			remote := sdk.NewServer(&sdk.Implementation{Name: "feature-owner"}, nil)
			backend, err := qimumcp.NewBackend(remote)
			if err != nil {
				t.Fatal(err)
			}
			for _, descriptor := range contract.Descriptor().Methods {
				method, err := qimumcp.ImplementMethod[jsonvalue.Value, jsonvalue.Value](contract, descriptor.Name)
				if err != nil {
					t.Fatal(err)
				}
				if err := method.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, qimumcp.Command[jsonvalue.Value]) (jsonvalue.Value, error) {
					return jsonvalue.Value{}, errors.New("fixture only executes the declared MCP tool")
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := backend.BindCapability("owner", contract); err != nil {
				t.Fatal(err)
			}
			if err := backend.Publish(); err != nil {
				t.Fatal(err)
			}
			if err := pluginsdk.RegisterTools(remote); err != nil {
				t.Fatal(err)
			}
			remote.AddTool(&sdk.Tool{Name: scenario.name, InputSchema: jsontext.Value(`{"type":"object"}`)}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				calls.Add(1)
				var invocation portable.Invocation
				if err := json.Unmarshal(request.Params.Meta[qimumcp.InvocationMetadataKey], &invocation); err != nil {
					return nil, err
				}
				if invocation.ID != "original-logical-call" || invocation.Origin != extension.Model || invocation.Parent != "run_owner" || invocation.Target.Reference != installation.ID().String() || string(request.Params.Meta["owner-meta"]) != `"retained"` {
					return nil, errors.New("original invocation metadata was lost")
				}
				if contract == memoryContracts {
					if err := json.Unmarshal(request.Params.Meta[pluginsdk.ToolContextMetadataKey], &captured); err != nil {
						return nil, err
					}
					if captured.CWD != "/captured/workspace" {
						return nil, errors.New("captured Memory workspace was changed")
					}
					if !scenario.embedding {
						if captured.EmbeddingSpace != "" {
							return nil, errors.New("memory management received an embedding space")
						}
						if _, err := qimumcp.CallbackFromContext(ctx); err == nil {
							return nil, errors.New("memory management received model authority")
						}
						return nil, errors.New("response unavailable after dispatch")
					}
					var err error
					capturedGrant, err = qimumcp.CallbackFromContext(ctx)
					if err != nil {
						return nil, err
					}
					if err := json.Unmarshal(request.Params.Meta[pluginsdk.ToolContextMetadataKey], &captured); err != nil {
						return nil, err
					}
					if captured.CWD != "/captured/workspace" || captured.EmbeddingSpace != embedder.ID() {
						return nil, errors.New("captured Memory scope was changed")
					}
					vectors, err := pluginsdk.CallTextEmbedding(ctx, capturedGrant, "prepared-embedding", []string{"rank"})
					if err != nil || len(vectors.Vectors) != 1 || vectors.Space != captured.EmbeddingSpace {
						return nil, errors.Join(err, errors.New("prepared callback failed"))
					}
				} else {
					ref, err := qimumcp.CallbackFromContext(ctx)
					if err != nil {
						return nil, err
					}
					if err := ref.Validate(); err != nil {
						return nil, err
					}
				}
				return nil, errors.New("response unavailable after dispatch")
			})
			transport := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
			t.Cleanup(transport.Close)
			server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}
			var embeddingFailure error
			resolve := func(context.Context) (models.TextEmbedder, error) {
				if embeddingFailure != nil {
					return nil, embeddingFailure
				}
				return embedder, nil
			}
			prepare, err := NewToolPreparation(ToolPreparationConfig{Installations: store, Callbacks: callbacks, Embeddings: resolve, Invalidations: func(notice invalidation.Notice) {
				if notice.Resource == invalidation.PluginData {
					notices.Add(1)
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			managed := newManagedFixture(t, store, server, callbacks, prepare)
			pool := managed.Pool
			var tools []toolcontract.Tool
			pool.SetToolSink(func(current []toolcontract.Tool, _ []mcpserver.ID) { tools = current }, prepare)
			if len(tools) != 1 {
				t.Fatal("source did not publish its tool")
			}
			bound, err := toolcontract.Bind(tools[0])
			if err != nil {
				t.Fatal(err)
			}
			invocation, err := bound.Contract().Prepare(chat.ToolCall{ID: "provider-call", Name: bound.Contract().Definition().Name, Arguments: `{}`}, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{WorkspaceCWD: "/captured/workspace"})
			ctx = operation.Adopt(ctx, "original-logical-call", extension.Model, "run_owner")
			var metadata sdk.Meta
			if err := metadata.Set("owner-meta", "retained"); err != nil {
				t.Fatal(err)
			}
			if contract == memoryContracts && !scenario.embedding {
				if err := metadata.Set(qimumcp.CallbackMetadataKey, map[string]string{"parent": "authority"}); err != nil {
					t.Fatal(err)
				}
			}
			ctx = scopemcp.WithRequestMeta(ctx, metadata)
			if _, err := bound.Call(ctx, invocation); err == nil || !strings.Contains(err.Error(), "response unavailable after dispatch") {
				t.Fatalf("prepared metadata/callback did not reach the dispatch boundary: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatal("prepared call did not reach its original owner")
			}
			if scenario.embedding {
				if embedder.calls.Load() != 1 || notices.Load() != 1 {
					t.Fatal("Memory callback or feature observation escaped its owner")
				}
				if _, err := pluginsdk.CallTextEmbedding(t.Context(), capturedGrant, "stale-embedding", []string{"stale"}); !errors.Is(err, qimumcp.ErrStaleCall) {
					t.Fatalf("failed Tool call retained its callback: %v", err)
				}
				embeddingFailure = errors.New("embedding source read failed")
				if _, err := bound.Call(ctx, invocation); !errors.Is(err, embeddingFailure) || calls.Load() != 1 {
					t.Fatalf("unexpected embedding failure silently fell back or reached dispatch: %v", err)
				}
			} else {
				if notices.Load() != 1 {
					t.Fatal("failed reply did not invalidate owner reads")
				}
				if contract == memoryContracts {
					embeddingFailure = errors.New("unused embedding source failed")
					if _, err := bound.Call(ctx, invocation); err == nil || !strings.Contains(err.Error(), "response unavailable after dispatch") || calls.Load() != 2 || embedder.calls.Load() != 0 {
						t.Fatalf("memory management depended on embeddings: %v", err)
					}
				}
			}
			store.failure = errors.New("installation read failed")
			before := calls.Load()
			_, err = bound.Call(ctx, invocation)
			failure, classified := toolcontract.ClassifyFailure(err)
			if !classified || !errors.Is(failure.Cause(), store.failure) || calls.Load() != before {
				t.Fatalf("unverifiable installed source lost its classified cause or dispatched: %v", err)
			}
		})
	}
}
