package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"iter"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// TestInstalledToolEntryReachesDeclaredAnalyzerDependency proves the model Tool
// entry now receives the same call-scoped host grant the public plugins.call
// entry receives. The reviews backend registers one analyzer business and exposes
// it as both a typed query and the reviews_analyze_review Tool; reached through the
// Tool, an ordinary (non-Memory) backend still obtains its CallbackFromContext
// grant and calls the same declared analyzer dependency. Before the installed grant
// reached ordinary Tools this backend's CallbackFromContext failed and the Tool
// produced no findings.
func TestInstalledToolEntryReachesDeclaredAnalyzerDependency(t *testing.T) {
	cfg := poMCPConfig(t)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("analyzed"), nil
		}
		const name = "reviews_analyze_review"
		conversation := withoutDeferredCatalog(request)
		for _, part := range conversation[len(conversation)-1].Parts {
			if part.ToolResult != nil && part.ToolResult.Name == name {
				return completedTextResponse("observed"), nil
			}
		}
		call := chat.ToolCall{ID: "analyze-call", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call.Name, call.Arguments = name, `{}`
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)

	activate := func(source, key string) *protocol.PluginInstallation {
		t.Helper()
		installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-"+key).(*protocol.PluginInstallation)
		return r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-"+key).(*protocol.PluginInstallation)
	}
	// The consumer freezes its analyzer dependency from the servers connected at
	// setup, so the dependency must be admitted before the reviews backend connects.
	waitConnected := func(installed *protocol.PluginInstallation, name string) {
		t.Helper()
		id := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: name}
		deadline := time.Now().Add(10 * time.Second)
		for {
			page := r.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
			for _, server := range page.Data {
				if server.ID == id && server.Status.Type == protocol.MCPServerConnected {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("installed author never connected: %s", name)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	analyzer := activate(reviewAuthorPackage(t, filepath.Join("analyzer", "package"), "review-analyzer"), "analyzer")
	waitConnected(analyzer, "analyzer")
	reviews := activate(reviewExamplePackage(t), "reviews")
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: reviews.ID}, Name: "reviews"}
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })

	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Analyzer tool entry"}, "session").(*protocol.Session)
	options := func(key string) delivery.Options {
		return delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}}}}
	}
	invoke := func(method delivery.Name, request any, key string) delivery.Result {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), method, request, options(key))
		if result.Failure != nil {
			t.Fatalf("%s: %v", method, result.Failure)
		}
		return result
	}
	drain := func(result delivery.Result) {
		t.Helper()
		events := iter.Seq2[protocol.RunEvent, error](func(yield func(protocol.RunEvent, error) bool) {
			for event, err := range result.Events {
				if err != nil {
					t.Errorf("event stream: %v", err)
					return
				}
				if !yield(event.(protocol.RunEvent), nil) {
					return
				}
			}
		})
		waitForRunEvents(t, collectRunEvents(events), "analyze")
	}

	started := invoke(delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Analyze the open review"}}}, "analyze-start")
	drain(started)
	runID := started.Value.(*protocol.StartRunResponse).RunID

	pending := invoke(delivery.InterruptsList, protocol.ListInterruptsRequest{RootRunID: runID}, "").Value.(*protocol.Page[protocol.PendingInterruptSet])
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 || pending.Data[0].Interrupts[0].Type != protocol.InterruptApproval {
		t.Fatalf("approval = %+v", pending.Data)
	}
	resume := protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{ItemID: pending.Data[0].Interrupts[0].ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove}}}}
	drain(invoke(delivery.RunsResume, resume, "analyze-approved"))

	items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID}}, "").(*protocol.ListItemsResponse)
	var analysis protocol.Item
	found := false
	for _, item := range items.Data {
		if item.Tool != nil && item.Tool.Name == "reviews_analyze_review" && item.Tool.Result != nil {
			analysis, found = item, true
			break
		}
	}
	if !found || analysis.Status != protocol.ItemStatusCompleted {
		t.Fatalf("analyzer Tool entry did not complete through its host grant: found=%t, %+v", found, analysis)
	}
	encoded, err := json.Marshal(analysis.Tool.Result)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Review struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"review"`
		Findings []struct {
			Code     string `json:"code"`
			ReviewID string `json:"reviewId"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.Review.ID != "example-review" || result.Review.Status != "open" || len(result.Findings) != 1 {
		t.Fatalf("Tool entry lost the declared analyzer dependency: %s", encoded)
	}
	if result.Findings[0].Code != "unresolvedReview" || result.Findings[0].ReviewID != result.Review.ID {
		t.Fatalf("analyzer finding differs from the inspected review: %s", encoded)
	}
}
