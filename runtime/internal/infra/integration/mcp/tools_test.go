package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type concurrencyPolicy interface {
	ConcurrencyPolicy() func(toolcontract.Invocation) (key string, concurrent bool)
}

type toolDecorator struct{ toolcontract.Tool }

func (t toolDecorator) Unwrap() toolcontract.Tool { return t.Tool }

func TestSourceToolsForwardsInvocationMetadataWithoutLeakingBetweenCalls(t *testing.T) {
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "metadata-test"}, nil)
	server.AddTool(&sdkmcp.Tool{Name: "update", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(_ context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{StructuredContent: request.Params.Meta}, nil
	})
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	tools, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("metadata")}, session)
	if err != nil || len(tools) != 1 {
		t.Fatalf("discovery = %+v, %v", tools, err)
	}
	binding, err := toolcontract.Bind(tools[0])
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "provider-id", Name: binding.Contract().Definition().Name, Arguments: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"logical-first", "logical-second", ""} {
		meta := map[string]any{}
		if id != "" {
			meta["io.github.tangerg.flame/invocationId"] = id
		}
		output, err := binding.Call(scopemcp.WithRequestMeta(t.Context(), meta), invocation)
		if err != nil {
			t.Fatal(err)
		}
		var received map[string]any
		if err := json.Unmarshal(output.Details, &received); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(received["io.github.tangerg.flame/invocationId"], meta["io.github.tangerg.flame/invocationId"]) {
			t.Fatalf("received invocation identity = %v, want %v", received, meta)
		}
	}
}

func TestSourceToolsDoesNotPublishPartialCatalogAfterScopeRejection(t *testing.T) {
	session := toolCatalogSession(t,
		&sdkmcp.Tool{Name: "read.file", InputSchema: jsontext.Value(`{"type":"object"}`)},
		&sdkmcp.Tool{Name: "read_file", InputSchema: jsontext.Value(`{"type":"object"}`)},
		&sdkmcp.Tool{Name: "other", InputSchema: jsontext.Value(`{"type":"object"}`)},
	)
	tools, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")}, session)
	if err == nil || len(tools) != 0 {
		t.Fatalf("Scope discovery rejection published a partial catalog: %v, %v", tools, err)
	}
}

func TestSourceToolsCompilesSchemasBeforePublishing(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"value":{"type":"unknown"}}}`,
		`{"type":"object","properties":{"value":{"$ref":"#/$defs/missing"}}}`,
		`{"type":"object","properties":{"value":{"$ref":"https://example.invalid/schema.json"}}}`,
	} {
		t.Run(schema, func(t *testing.T) {
			session := toolCatalogSession(t, &sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(schema)})
			tools, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")}, session)
			if len(tools) != 0 || !errors.Is(err, toolcontract.ErrInvalidTool) || !errors.Is(err, jsonschema.ErrInvalid) {
				t.Fatalf("sourceTools = %v, %v; want Scope schema admission failure", tools, err)
			}
		})
	}
}

func TestSourceToolsEnablesOnlyAnnotatedReadOnlyConcurrencyPolicy(t *testing.T) {
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	for _, descriptor := range []*sdkmcp.Tool{
		{
			Name:        "lookup",
			InputSchema: jsontext.Value(`{"type":"object"}`),
			Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
		},
		{
			Name:        "mutate",
			InputSchema: jsontext.Value(`{"type":"object"}`),
		},
	} {
		server.AddTool(descriptor, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{}, nil
		})
	}
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	wrapped, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")}, clientSession)
	if err != nil {
		t.Fatalf("sourceTools: %v", err)
	}
	if len(wrapped) != 2 {
		t.Fatalf("sourceTools count = %d, want 2", len(wrapped))
	}

	got := make(map[string]bool, len(wrapped))
	for _, tool := range wrapped {
		ref, found, err := IdentifyTool(toolDecorator{Tool: toolDecorator{Tool: tool}})
		server, remote, isMCP := ref.MCP()
		if err != nil || !found || !isMCP || server != testsupport.UserMCPServer("catalog") ||
			(remote.String() != "lookup" && remote.String() != "mutate") {
			t.Fatalf("decorated Scope Tool identity = %+v, %t, %v", ref, found, err)
		}
		keyer, ok, err := toolcontract.Capability[concurrencyPolicy](tool)
		if err != nil || !ok {
			t.Fatalf("tool %q does not expose concurrency policy", tool.Definition().Name)
		}
		binding, err := toolcontract.Bind(tool)
		if err != nil {
			t.Fatal(err)
		}
		invocation, err := binding.Contract().Prepare(chat.ToolCall{
			ID: "test_call", Name: binding.Contract().Definition().Name, Arguments: `{"id":"one"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		key, concurrent := keyer.ConcurrencyPolicy()(invocation)
		if key != "" {
			t.Fatalf("tool %q concurrency key = %q, want empty", tool.Definition().Name, key)
		}
		got[tool.Definition().Name] = concurrent
	}
	if !got["catalog_lookup"] || got["catalog_mutate"] {
		t.Fatalf("source tool concurrency = %v, want lookup=true mutate=false", got)
	}
}

func TestRemoteToolCatalogRejectsUnboundedMaterial(t *testing.T) {
	t.Run("model-facing description", func(t *testing.T) {
		session := toolCatalogSession(t, &sdkmcp.Tool{
			Name:        "oversized-description",
			Description: strings.Repeat("x", mcpserver.MaxRemoteToolDescriptionBytes+1),
			InputSchema: jsontext.Value(`{"type":"object"}`),
		})
		if _, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")}, session); err == nil {
			t.Fatal("sourceTools accepted a description larger than 64 KiB")
		}
	})

	t.Run("connection schema", func(t *testing.T) {
		session := toolCatalogSession(t, &sdkmcp.Tool{
			Name: "oversized-schema",
			InputSchema: jsontext.Value(`{"type":"object","description":"` +
				strings.Repeat("x", (1<<20)+1) + `"}`),
		})
		if _, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")}, session); err == nil {
			t.Fatal("sourceTools accepted a schema larger than Scope's 1 MiB bound")
		}
	})

	t.Run("tool count", func(t *testing.T) {
		candidate := make([]Executable, mcpserver.MaxRemoteToolsPerServer+1)
		for index := range candidate {
			candidate[index] = Executable{Tool: catalogTool(fmt.Sprintf("catalog_tool_%04d", index))}
		}
		if err := validateSourceToolMaterial(testsupport.UserMCPServer("catalog"), candidate); err == nil {
			t.Fatal("source catalog accepted more than 2,048 remote tools")
		}
	})
}

func toolCatalogSession(t *testing.T, descriptors ...*sdkmcp.Tool) *sdkmcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	for _, descriptor := range descriptors {
		server.AddTool(descriptor, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{}, nil
		})
	}
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func TestSourceToolsPreservesStructuredResultThroughTransport(t *testing.T) {
	for _, payload := range []string{`{"decimal":0.123456789012345678901,"integer":18446744073709551615}`, `{}`, `null`} {
		t.Run(payload, func(t *testing.T) {
			serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
			server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "exact-results"}, nil)
			server.AddTool(&sdkmcp.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
				return &sdkmcp.CallToolResult{StructuredContent: jsontext.Value(payload)}, nil
			})
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer serverSession.Close()
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime"}, nil)
			session, err := client.Connect(t.Context(), clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			tools, err := sourceTools(t.Context(), nil, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")}, session)
			if err != nil {
				t.Fatal(err)
			}
			if len(tools) != 1 {
				t.Fatalf("tools = %d", len(tools))
			}
			binding, err := toolcontract.Bind(tools[0])
			if err != nil {
				t.Fatal(err)
			}
			invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "exact", Name: binding.Contract().Definition().Name, Arguments: `{}`})
			if err != nil {
				t.Fatal(err)
			}
			output, err := binding.Call(t.Context(), invocation)
			if err != nil {
				t.Fatal(err)
			}
			// JSON member order is transport-owned; number lexemes and explicit
			// empty objects must survive without float64 conversion.
			var got, want map[string]jsontext.Value
			if err := json.Unmarshal(output.Details, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(payload), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("structured result = %s, want %s", output.Details, payload)
			}
		})
	}
}

// Equal local names under different origins are different servers; the tool
// reference takes its server from the connection, never from Scope's label.
func TestSourceToolsCarryTheRealizedServerIdentity(t *testing.T) {
	installation, err := resourceid.ParseInstallation("eeb329cd-c7ce-40c9-bd90-6821fef06d30")
	if err != nil {
		t.Fatal(err)
	}
	source, err := mcpserver.InstallationSource(installation, fingerprint.Strings("release"), fingerprint.Strings("authority"), fingerprint.Strings("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	for _, config := range []ServerConfig{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("catalog")},
		{Source: source, Name: testsupport.ServerName("catalog")},
	} {
		session := toolCatalogSession(t, &sdkmcp.Tool{Name: "lookup", InputSchema: jsontext.Value(`{"type":"object"}`)})
		tools, err := sourceTools(t.Context(), nil, config, session)
		if err != nil || len(tools) != 1 {
			t.Fatalf("sourceTools = %d, %v", len(tools), err)
		}
		ref, _, err := IdentifyTool(tools[0])
		server, _, _ := ref.MCP()
		if err != nil || server != config.ID() || ref.ModelName() != "catalog_lookup" {
			t.Fatalf("tool identity = %v, %v; want server %v", ref, err, config.ID())
		}
	}
}
