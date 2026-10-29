package bootstrap

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/tools/web"
)

func TestWebInstalledSecretsBelongToInstallationAndModelReadsHaveDefiniteRefusals(t *testing.T) {
	t.Setenv("JINA_API_KEY", "parent-jina-fixture-key")
	t.Setenv("TAVILY_API_KEY", "parent-tavily-fixture-key")
	var name, failureText atomic.Value
	name.Store("")
	failureText.Store("")
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		target := name.Load().(string)
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == target {
					if !part.ToolResult.IsError || len(part.ToolResult.Output.Content) != 1 {
						t.Error("unconfigured read lost its refusal")
					} else {
						failureText.Store(part.ToolResult.Output.Content[0].Text)
					}
					return completedTextResponse("search is not configured"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "discover-web", Name: "search_tools", Arguments: `{"query":"select:` + target + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == target {
				call = chat.ToolCall{ID: "search-web", Name: target, Arguments: `{"query":"fixture query","max_results":1}`}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.chatResolver = testChatResolver(client)
	cfg.approvalMode = approval.ModeYolo
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "web"))
	if len(installed.Selected.Inputs) != 3 {
		t.Fatal("web credential inputs are missing")
	}
	for _, input := range installed.Selected.Inputs {
		if (input.ID != "http-allowed-hosts" && !input.Secret) || input.Required {
			t.Fatal("web credentials do not use optional private inputs")
		}
	}
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "web"}
	tools := r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 4 })
	for _, tool := range tools {
		if tool.Name == "web_search" {
			name.Store(tool.ModelName)
		}
	}
	if name.Load().(string) == "" {
		t.Fatal("installed search tool is missing")
	}
	contracts, err := portable.ParseCapability([]byte(testsupport.WebCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode(web.SearchRequest{Query: "fixture query", MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	typed := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "web-operations", Contract: contracts.Identity().String(), Method: testsupport.WebSearchV1Method, Input: input.String(), Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}, "").(*protocol.CallPluginResponse)
	if typed.Failure == nil || typed.Failure.Code != "search_not_configured" {
		t.Fatalf("parent credentials became installation authority: %+v", typed)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Web search"}, "web-session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Search for the fixture query."}}}, delivery.Options{IdempotencyKey: "web-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.Value.(*protocol.StartRunResponse).RunID}, "").(*protocol.RunRef)
	if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted || failureText.Load().(string) != typed.Failure.Message {
		t.Fatalf("installed refusal parity: %+v typed=%+v model=%s", run, typed.Failure, failureText.Load())
	}
}
