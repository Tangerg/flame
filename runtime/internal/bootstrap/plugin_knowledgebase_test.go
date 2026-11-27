package bootstrap

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

func TestKnowledgeBaseInstalledEntriesShareRetrievalAndLeaveConversationWithCore(t *testing.T) {
	kb := &knowledgeProvider{t: t, referenced: map[string]string{}, retrieved: map[string]string{}}
	kb.toolName.Store("")
	kb.toolOutput.Store("")
	provider := httptest.NewServer(kb)
	t.Cleanup(provider.Close)
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection("openai-compatible", "knowledge-model")
	r := poMCPOpen(t, cfg)
	baseURL, key := provider.URL+"/v1", "local-embedding-fixture"
	activateModelProviders(r)
	if _, err := r.host.application.delivery.Models.ListProviders(t.Context()); err != nil {
		t.Fatalf("installed provider catalog: %v", err)
	}
	r.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{Provider: "openai-compatible", BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &baseURL}, APIKey: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &key}}, "configure-chat")
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
			kb.toolName.Store(tool.ModelName)
		}
	}
	if kb.toolName.Load().(string) == "" {
		t.Fatal("knowledge search Tool missing")
	}
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.KnowledgebaseCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any, key string) *protocol.CallPluginResponse {
		t.Helper()
		admitted, err := framevalue.EncodeJSON(input)
		if err != nil {
			t.Fatal(err)
		}
		response := r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: installed.ID,
				Digest:         installed.Selected.Digest,
				Contribution:   "knowledgebase-operations",
				Contract:       contracts.Identity().String(),
				Method:         method,
				Target:         framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
			},
			Input: admitted.String(),
		}, key).(*protocol.CallPluginResponse)
		if response.Failure != nil {
			t.Fatalf("knowledge operation: %+v", response.Failure)
		}
		return response
	}
	input := testsupport.KnowledgebaseImportInput{Content: &testsupport.KnowledgebaseContent{}, ExpectedRevision: generatedNumber(t, "0")}
	input.Content.Id, input.Content.Title = "fruit", "Fruit guide"
	input.Content.Source, input.Content.Text = "docs/fruit.md", "apple guide"
	first := call(testsupport.KnowledgebaseImportV1Method, input, "knowledge-import")
	if replay := call(testsupport.KnowledgebaseImportV1Method, input, "knowledge-import"); replay.Output != first.Output || kb.embeddings.Load() != 1 {
		t.Fatalf("import repeated its model exchange: %+v, count=%d", replay, kb.embeddings.Load())
	}
	typed := call(testsupport.KnowledgebaseSearchV1Method, testsupport.KnowledgebaseSearchInput{Query: "apple", Limit: generatedNumber(t, "2")}, "")
	contextContract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	r.must(delivery.PluginsSetComposition, protocol.SetPluginCompositionRequest{Contract: contextContract.Identity().String(), Members: []protocol.PluginCompositionMember{{InstallationID: installed.ID, Contribution: "knowledgebase-context"}}}, "select-knowledge-context")
	runTask := func(project, key, task string) *protocol.Session {
		t.Helper()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: task, ProjectID: project}, "session-"+key).(*protocol.Session)
		run := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: task}}}, protocol.CallMetadata{IdempotencyKey: "run-" + key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
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
		return session
	}
	session := runTask("", "apple-guide", "Find the apple guide.")
	toolJSON, err := framevalue.ParseJSON([]byte(kb.toolOutput.Load().(string)))
	if err != nil || toolJSON.String() != typed.Output || !kb.sawContext.Load() || !kb.embeddedUserInput.Load() {
		t.Fatalf("installed typed/Tool/context entrances diverged: typed=%s tool=%s context=%v error=%v", typed.Output, toolJSON.String(), kb.sawContext.Load(), err)
	}
	items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}}, "")
	encoded, err := json.Marshal(items)
	if err != nil || strings.Contains(string(encoded), "Retrieved document excerpts") || strings.Contains(string(encoded), "Runtime reference material") {
		t.Fatalf("context contribution became authored history: %s, %v", encoded, err)
	}
	beforeRead := kb.embeddings.Load()
	read := call(testsupport.KnowledgebaseReadV1Method, testsupport.KnowledgebaseReadInput{Id: "fruit"}, "")
	var original testsupport.KnowledgebaseDocumentRead
	if err := json.Unmarshal([]byte(read.Output), &original); err != nil || original.Text != input.Content.Text || kb.embeddings.Load() != beforeRead {
		t.Fatalf("installed original read performed embedding or lost text: %+v %v", original, err)
	}
	reindex := testsupport.KnowledgebaseReindexV1InputWireDocumentInput{Id: "fruit", ExpectedRevision: original.Document.Revision}
	rebuilt := call(testsupport.KnowledgebaseReindexV1Method, reindex, "knowledge-reindex")
	if replay := call(testsupport.KnowledgebaseReindexV1Method, reindex, "knowledge-reindex"); replay.Output != rebuilt.Output || kb.embeddings.Load() != beforeRead+1 {
		t.Fatalf("installed reindex repeated embedding: %+v count=%d", replay, kb.embeddings.Load())
	}

	// A Project's document reaches only that Project's Runs, through both the
	// automatic context and the model's own search.
	orchard := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Orchard"}, "orchard").(*protocol.Project)
	scoped := testsupport.KnowledgebaseImportInput{Content: &testsupport.KnowledgebaseContent{Project: &orchard.ID}, ExpectedRevision: generatedNumber(t, "0")}
	scoped.Content.Id, scoped.Content.Title = "ledger", "Orchard ledger"
	scoped.Content.Source, scoped.Content.Text = "docs/ledger.md", "apple ledger"
	call(testsupport.KnowledgebaseImportV1Method, scoped, "knowledge-import-ledger")
	runTask("", "shared-apple-notes", "Find the shared apple notes.")
	runTask(orchard.ID, "orchard-apple-notes", "Find the orchard apple notes.")
	kb.mu.Lock()
	defer kb.mu.Unlock()
	for task, wantLedger := range map[string]bool{"Find the shared apple notes.": false, "Find the orchard apple notes.": true} {
		for source, seen := range map[string]string{"context": kb.referenced[task], "search": kb.retrieved[task]} {
			if !strings.Contains(seen, "docs/fruit.md") || strings.Contains(seen, "docs/ledger.md") != wantLedger {
				t.Fatalf("%q %s crossed its Project boundary: %s", task, source, seen)
			}
		}
	}
}

func knowledgeChatText(value jsontext.Value) (string, error) {
	if value.Kind() == '"' {
		var text string
		err := json.Unmarshal(value, &text)
		return text, err
	}
	if value.Kind() != '[' {
		return "", fmt.Errorf("unexpected chat content kind %q", value.Kind())
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(value, &parts); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, part := range parts {
		if part.Type != "text" {
			return "", fmt.Errorf("unexpected chat content type %q", part.Type)
		}
		text.WriteString(part.Text)
	}
	return text.String(), nil
}

// knowledgeProvider is the OpenAI-compatible provider of the knowledge base
// scenario: chat turns first discover and then call the installed retrieval
// tool, and every turn records the reference material and retrieved results
// it was given, keyed by the user task.
type knowledgeProvider struct {
	t                 *testing.T
	embeddings        atomic.Int32
	embeddedUserInput atomic.Bool
	toolName          atomic.Value
	sawContext        atomic.Bool
	toolOutput        atomic.Value
	mu                sync.Mutex
	referenced        map[string]string
	retrieved         map[string]string
}

type knowledgeChatRequest struct {
	Messages []struct {
		Role       string         `json:"role"`
		Content    jsontext.Value `json:"content"`
		ToolCallID string         `json:"tool_call_id"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

func (p *knowledgeProvider) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch {
	case strings.HasSuffix(request.URL.Path, "/chat/completions"):
		p.serveChat(w, request)
	case strings.HasSuffix(request.URL.Path, "/embeddings"):
		p.serveEmbeddings(w, request)
	default:
		http.NotFound(w, request)
	}
}

func (p *knowledgeProvider) serveChat(w http.ResponseWriter, request *http.Request) {
	var input knowledgeChatRequest
	if err := json.UnmarshalRead(request.Body, &input); err != nil {
		p.t.Error(err)
		http.Error(w, "invalid chat", 400)
		return
	}
	finish, text := "stop", "knowledge test"
	var call map[string]any
	if len(input.Tools) > 0 {
		retrieved, err := p.recordTurn(input)
		if err != nil {
			p.t.Error(err)
			http.Error(w, "invalid content", http.StatusBadRequest)
			return
		}
		if retrieved {
			text = "cited fruit guide"
		} else {
			call, finish = p.nextCall(input), "tool_calls"
		}
	}
	delta := map[string]any{"role": "assistant", "content": text}
	if call != nil {
		delta = map[string]any{"role": "assistant", "tool_calls": []any{call}}
	}
	frame := map[string]any{"id": "kb-completion", "model": "knowledge-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}}
	w.Header().Set("Content-Type", "text/event-stream")
	encoded, err := json.Marshal(frame)
	if err != nil {
		p.t.Error(err)
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
}

// recordTurn keeps what the model was given under its user task and reports
// whether the retrieval tool's result has arrived.
func (p *knowledgeProvider) recordTurn(input knowledgeChatRequest) (bool, error) {
	task, retrieved := "", false
	for _, message := range input.Messages {
		if message.Role != "user" && message.Role != "tool" {
			continue
		}
		content, err := knowledgeChatText(message.Content)
		if err != nil {
			return false, err
		}
		if message.Role == "user" && task == "" {
			task = content
		}
		if message.Role == "user" && strings.Contains(content, "Runtime reference material") {
			p.mu.Lock()
			p.referenced[task] += content
			p.mu.Unlock()
			if strings.Contains(content, "docs/fruit.md") {
				p.sawContext.Store(true)
			}
		}
		if message.Role == "tool" && message.ToolCallID == "retrieve-knowledge" {
			p.mu.Lock()
			p.retrieved[task] = content
			p.mu.Unlock()
			p.toolOutput.Store(content)
			retrieved = true
		}
	}
	return retrieved, nil
}

// nextCall discovers the retrieval tool until it is offered, then calls it.
func (p *knowledgeProvider) nextCall(input knowledgeChatRequest) map[string]any {
	name := p.toolName.Load().(string)
	id, selected, arguments := "discover-knowledge", "search_tools", `{"query":"select:`+name+`"}`
	for _, definition := range input.Tools {
		if definition.Function.Name == name {
			id, selected, arguments = "retrieve-knowledge", name, `{"query":"apple","limit":2}`
		}
	}
	return map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]string{"name": selected, "arguments": arguments}}
}

func (p *knowledgeProvider) serveEmbeddings(w http.ResponseWriter, request *http.Request) {
	var input struct {
		Input []string `json:"input"`
	}
	if err := json.UnmarshalRead(request.Body, &input); err != nil {
		p.t.Errorf("embedding request: %v", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	p.embeddings.Add(1)
	for _, text := range input.Input {
		if text == "Find the apple guide." {
			p.embeddedUserInput.Store(true)
		}
	}
	data := make([]map[string]any, len(input.Input))
	for i := range data {
		data[i] = map[string]any{"object": "embedding", "index": i, "embedding": []float64{1, 0}}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.MarshalWrite(w, map[string]any{"object": "list", "model": "text-embedding-3-small", "data": data, "usage": map[string]int{"prompt_tokens": 5, "total_tokens": 5}}); err != nil {
		p.t.Error(err)
	}
}
