package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestKnowledgeBaseInstalledEntriesShareRetrievalAndLeaveConversationWithCore(t *testing.T) {
	var embeddings atomic.Int32
	var embeddedUserInput atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/embeddings") {
			http.NotFound(w, request)
			return
		}
		var input struct {
			Input []string `json:"input"`
		}
		if err := json.UnmarshalRead(request.Body, &input); err != nil {
			t.Errorf("embedding request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		embeddings.Add(1)
		for _, text := range input.Input {
			if text == "Find the apple guide." {
				embeddedUserInput.Store(true)
			}
		}
		data := make([]map[string]any, len(input.Input))
		for i := range data {
			data[i] = map[string]any{"object": "embedding", "index": i, "embedding": []float64{1, 0}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{"object": "list", "model": "text-embedding-3-small", "data": data, "usage": map[string]int{"prompt_tokens": 5, "total_tokens": 5}}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(provider.Close)
	var toolName atomic.Value
	toolName.Store("")
	var sawContext atomic.Bool
	var toolOutput atomic.Value
	toolOutput.Store("")
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("knowledge test"), nil
		}
		name := toolName.Load().(string)
		for _, message := range request.Messages {
			if message.Role == chat.RoleSystem && strings.Contains(message.Text(), "docs/fruit.md") {
				sawContext.Store(true)
			}
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == name {
					toolOutput.Store(string(part.ToolResult.Output.Details))
					return completedTextResponse("cited fruit guide"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "discover-knowledge", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call = chat.ToolCall{ID: "retrieve-knowledge", Name: name, Arguments: `{"query":"apple","limit":1}`}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	cfg.chatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	baseURL, key := provider.URL+"/v1", "local-embedding-fixture"
	r.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{Provider: "openai", BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &baseURL}, APIKey: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &key}}, "configure-embedding")
	r.must(delivery.ModelsSetEmbeddingRole, protocol.EmbeddingRole{Provider: "openai", Model: "text-embedding-3-small"}, "select-embedding")
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "knowledgebase"))
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Contributions) != 2 {
		t.Fatalf("knowledge package was not admitted: %+v", installed.Selected)
	}
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "knowledgebase"}
	tools := r.poMCPTools(server, func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "search_knowledge" }) &&
			slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "read_knowledge_document" })
	})
	for _, tool := range tools {
		if tool.Name == "search_knowledge" {
			toolName.Store(tool.ModelName)
		}
	}
	if toolName.Load().(string) == "" {
		t.Fatal("knowledge search Tool missing")
	}
	contracts, err := portable.ParseCapability([]byte(testsupport.KnowledgebaseCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any, key string) *protocol.CallPluginResponse {
		t.Helper()
		admitted, err := jsonvalue.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		response := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "knowledgebase-operations", Contract: contracts.Identity().String(), Method: method, Input: admitted.String(), Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}, key).(*protocol.CallPluginResponse)
		if response.Failure != nil {
			t.Fatalf("knowledge operation: %+v", response.Failure)
		}
		return response
	}
	input := testsupport.KnowledgebaseImportV1Input{}
	input.Content.Id, input.Content.Title = "fruit", "Fruit guide"
	input.Content.Source, input.Content.Text = "docs/fruit.md", "apple guide"
	first := call(testsupport.KnowledgebaseImportV1Method, input, "knowledge-import")
	if replay := call(testsupport.KnowledgebaseImportV1Method, input, "knowledge-import"); replay.Output != first.Output || embeddings.Load() != 1 {
		t.Fatalf("import repeated its model exchange: %+v, count=%d", replay, embeddings.Load())
	}
	typed := call(testsupport.KnowledgebaseSearchV1Method, testsupport.KnowledgebaseSearchV1Input{Query: "apple", Limit: 1}, "")
	contextContract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	r.must(delivery.PluginsSetComposition, protocol.SetPluginCompositionRequest{Contract: contextContract.Identity().String(), Members: []protocol.PluginCompositionMember{{InstallationID: installed.ID, Contribution: "knowledgebase-context"}}}, "select-knowledge-context")
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "knowledge"}, "knowledge-session").(*protocol.Session)
	run := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Find the apple guide."}}}, delivery.Options{IdempotencyKey: "knowledge-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if run.Failure != nil {
		t.Fatal(run.Failure)
	}
	for _, err := range run.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	started := run.Value.(*protocol.StartRunResponse)
	finished := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("knowledge Run did not complete: %+v", finished.Outcome)
	}
	toolJSON, err := jsonvalue.Parse([]byte(toolOutput.Load().(string)))
	if err != nil || toolJSON.String() != typed.Output || !sawContext.Load() || !embeddedUserInput.Load() {
		t.Fatalf("installed typed/Tool/context entrances diverged: typed=%s tool=%s context=%v error=%v", typed.Output, toolJSON.String(), sawContext.Load(), err)
	}
	items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}}, "")
	encoded, err := json.Marshal(items)
	if err != nil || strings.Contains(string(encoded), "Retrieved user reference material") {
		t.Fatalf("context contribution became authored history: %s, %v", encoded, err)
	}
	beforeRead := embeddings.Load()
	read := call(testsupport.KnowledgebaseReadV1Method, testsupport.KnowledgebaseReadV1Input{Id: "fruit"}, "")
	var original testsupport.KnowledgebaseReadV1Output
	if err := json.Unmarshal([]byte(read.Output), &original); err != nil || original.Text != input.Content.Text || embeddings.Load() != beforeRead {
		t.Fatalf("installed original read performed embedding or lost text: %+v %v", original, err)
	}
	reindex := testsupport.KnowledgebaseReindexV1Input{Id: "fruit", ExpectedRevision: uint64(original.Document.Revision)}
	rebuilt := call(testsupport.KnowledgebaseReindexV1Method, reindex, "knowledge-reindex")
	if replay := call(testsupport.KnowledgebaseReindexV1Method, reindex, "knowledge-reindex"); replay.Output != rebuilt.Output || embeddings.Load() != beforeRead+1 {
		t.Fatalf("installed reindex repeated embedding: %+v count=%d", replay, embeddings.Load())
	}
}
