package bootstrap

import (
	"context"
	"encoding/json/jsontext"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestMCPNotificationUpdatesManagementAndSubsequentRunDiscovery(t *testing.T) {
	remote := sdk.NewServer(&sdk.Implementation{Name: "catalog"}, nil)
	result := func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	}
	remote.AddTool(&sdk.Tool{Name: "read", InputSchema: jsontext.Value(`{"type":"object"}`)}, result)
	transport := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	var target atomic.Value
	target.Store("catalog_read")
	var seen sync.Map
	var mu sync.Mutex
	attempts := make(map[string]int)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		name := target.Load().(string)
		if name == "catalog_write" && slices.ContainsFunc(request.Tools, func(definition chat.ToolDefinition) bool { return definition.Name == "catalog_read" }) {
			return nil, fmt.Errorf("removed tool was advertised to the next Run")
		}
		if slices.ContainsFunc(request.Tools, func(definition chat.ToolDefinition) bool { return definition.Name == name }) {
			seen.Store(name, true)
			return completedTextResponse("discovered " + name), nil
		}
		mu.Lock()
		attempts[name]++
		count := attempts[name]
		mu.Unlock()
		if count > 3 {
			return nil, fmt.Errorf("target %q was not discovered", name)
		}
		call := chat.ToolCall{ID: fmt.Sprintf("discover-%d", count), Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.chatResolver = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	server := runtime.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: "catalog", Enabled: true, HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
		Connection: protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: transport.URL},
	}, "create-catalog").(*protocol.MCPServer)
	ready := func(name string) {
		t.Helper()
		runtime.poMCPTools(&server.ID, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == name })
	}
	ready("read")
	session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "changing tools"}, "create-session").(*protocol.Session)
	ctx := protocolLifecycleContext(t.Context())
	execute := func(expected string) {
		t.Helper()
		started, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Discover " + expected}}})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), expected)
		finished, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
		if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("discovery Run = %+v, %v", finished, err)
		}
		if _, found := seen.Load(expected); !found {
			t.Fatalf("new Run never received %q from the current manifest", expected)
		}
	}
	execute("catalog_read")
	remote.RemoveTools("read")
	remote.AddTool(&sdk.Tool{Name: "write", InputSchema: jsontext.Value(`{"type":"object"}`)}, result)
	ready("write")
	target.Store("catalog_write")
	execute("catalog_write")
}
