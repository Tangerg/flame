package bootstrap

import (
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

func TestCodeModeMatchesDirectReadsAcrossWorkspaceAndKnowledgeBase(t *testing.T) {
	provider := &codeModeComparisonProvider{t: t}
	server := httptest.NewServer(http.HandlerFunc(provider.serve))
	t.Cleanup(server.Close)
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection("openai-compatible", "knowledge-model")
	r := poMCPOpen(t, cfg)
	activateModelProviders(r)
	if _, err := r.host.application.delivery.Models.ListProviders(t.Context()); err != nil {
		t.Fatal(err)
	}
	base, key := server.URL+"/v1", "isolated-comparison"
	for _, name := range []string{"openai-compatible", "openai"} {
		r.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{Provider: name,
			BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &base},
			APIKey:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &key},
		}, "comparison-"+name)
	}
	r.must(delivery.ModelsSetEmbeddingRole, protocol.EmbeddingRole{Provider: "openai", Model: "text-embedding-3-small"}, "comparison-embedding")
	kb := poMCPActivate(r, filepath.Join(firstPartyPackages, "knowledgebase"))
	read := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: kb.ID}, Name: "knowledgebase"}
	for _, tool := range r.poMCPTools(read, func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "read_knowledge_document" })
	}) {
		if tool.Name == "read_knowledge_document" {
			provider.readName = tool.ModelName
		}
	}
	contract, err := plugincontract.ParseCapability([]byte(testsupport.KnowledgebaseCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	const firstID = 9007199254740993
	lines := func(start int) string {
		var text strings.Builder
		for index := start; index < start+80; index++ {
			category := "P2"
			if index%2 == 0 {
				category = "P1"
			}
			fmt.Fprintf(&text, "%d,%s,%s\n", firstID+int64(index), category, "sample delivery event with retained source and verification note")
		}
		return text.String()
	}
	file, material := lines(0), lines(40)
	if err := os.WriteFile(filepath.Join(cfg.defaultWorkspacePath, "records.csv"), []byte(file), 0600); err != nil {
		t.Fatal(err)
	}
	source, err := framevalue.EncodeJSON(map[string]any{"expectedRevision": 0, "content": map[string]any{"id": "records", "title": "Archived events", "source": "archived-events.csv", "text": material}})
	if err != nil {
		t.Fatal(err)
	}
	imported := r.must(delivery.PluginsCall, protocol.CallPluginRequest{
		Route: protocol.PluginCallRoute{
			InstallationID: kb.ID, Digest: kb.Selected.Digest, Contribution: "knowledgebase-operations",
			Contract: contract.Identity().String(), Method: testsupport.KnowledgebaseImportV1Method,
			Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: kb.ID},
		},
		Input: source.String(),
	}, "comparison-source").(*protocol.CallPluginResponse)
	if imported.Failure != nil {
		t.Fatal(imported.Failure)
	}

	durations := make([]time.Duration, 0, 2)
	var scriptRun string
	for variant, key := range []string{"direct", "code"} {
		provider.mu.Lock()
		provider.variant = variant
		provider.mu.Unlock()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Compare sources", Workspace: &protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}}, "compare-session-"+key).(*protocol.Session)
		startedAt := time.Now()
		result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Read records.csv and page 1 of the archived events. Count unique P1 IDs and return coverage and the greatest exact ID."}},
		}, protocol.CallMetadata{IdempotencyKey: "compare-run-" + key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		for _, err := range result.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		runID := result.Value.(*protocol.StartRunResponse).RunID
		durations = append(durations, time.Since(startedAt))
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
		if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("%s Read Run failed: %+v", key, run.Outcome)
		}
		if variant == 1 {
			scriptRun = runID
		}
	}
	provider.mu.Lock()
	results, dataBytes, requestBytes := provider.results, provider.dataBytes, provider.requestBytes
	provider.mu.Unlock()
	want := comparedReads{Expected: 2, Read: 2, Unique: 60, Greatest: "9007199254741111"}
	if results[0] != want || results[1] != want {
		t.Fatalf("summary changed across routes: direct=%+v code=%+v want=%+v", results[0], results[1], want)
	}
	if dataBytes[0] <= 0 || dataBytes[1] <= 0 || dataBytes[1] >= dataBytes[0] {
		t.Fatalf("model-visible data did not shrink: %+v bytes", dataBytes)
	}
	t.Logf("same task and coverage 2/2, unique P1=60, greatest exact ID=%s; model-visible read results %d → %d UTF-8 bytes; total chat request bodies %d → %d bytes; elapsed %s → %s", want.Greatest, dataBytes[0], dataBytes[1], requestBytes[0], requestBytes[1], durations[0], durations[1])
	items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: scriptRun}}, "").(*protocol.ListItemsResponse)
	for _, item := range items.Data {
		if item.Tool == nil || item.Tool.Name != string(builtin.RunScript) {
			continue
		}
		encoded, err := json.Marshal(item.Tool.Result)
		if err != nil {
			t.Fatal(err)
		}
		var evidence builtin.ScriptResult
		if err := json.Unmarshal(encoded, &evidence); err != nil || evidence.Status != builtin.ScriptCompleted || len(evidence.Calls) != 2 {
			t.Fatalf("retained child evidence: %s, %v", encoded, err)
		}
		for _, call := range evidence.Calls {
			expected, field := file, "content"
			if call.Tool == provider.readName {
				expected, field = material, "text"
			}
			values, ok := call.Result.(map[string]any)
			if !ok || strings.TrimSpace(fmt.Sprint(values[field])) != strings.TrimSpace(expected) ||
				call.Status != builtin.ScriptCallSucceeded || call.Source == "" {
				t.Fatalf("child read lost its original evidence: %s %s %s", call.Tool, call.Status, call.Source)
			}
		}
		return
	}
	t.Fatal("Code Mode result is missing from the original Run")
}

type comparedReads struct {
	Expected int    `json:"expected"`
	Read     int    `json:"read"`
	Unique   int    `json:"unique"`
	Greatest string `json:"greatest"`
}

type codeModeComparisonProvider struct {
	t            *testing.T
	readName     string
	mu           sync.Mutex
	variant      int
	results      [2]comparedReads
	dataBytes    [2]int
	requestBytes [2]int
}

func (p *codeModeComparisonProvider) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/embeddings") {
		(&knowledgeProvider{t: p.t}).serveEmbeddings(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.t.Error(err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var input knowledgeChatRequest
	if err := json.Unmarshal(body, &input); err != nil {
		p.t.Error(err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	delta, finish := map[string]any{"role": "assistant", "content": "read comparison"}, "stop"
	if len(input.Tools) > 0 {
		p.mu.Lock()
		p.requestBytes[p.variant] += len(body)
		calls, err := p.next(input)
		p.mu.Unlock()
		if err != nil {
			p.t.Error(err)
			http.Error(w, "invalid tool result", http.StatusBadRequest)
			return
		}
		if len(calls) > 0 {
			delta = map[string]any{"role": "assistant", "tool_calls": calls}
			finish = "tool_calls"
		}
	}
	frame, err := json.Marshal(map[string]any{
		"id": "compared-reads", "model": "knowledge-model",
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
	})
	if err != nil {
		p.t.Error(err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", frame)
}

func comparisonCall(index int, id, name, args string) any {
	return map[string]any{"index": index, "id": id, "type": "function", "function": map[string]string{"name": name, "arguments": args}}
}

func (p *codeModeComparisonProvider) next(input knowledgeChatRequest) ([]any, error) {
	results := map[string]string{}
	for _, message := range input.Messages {
		if message.Role != "tool" {
			continue
		}
		text, err := knowledgeChatText(message.Content)
		if err != nil {
			return nil, err
		}
		results[message.ToolCallID] = text
	}
	if file, kb := results["file"], results["kb"]; file != "" && kb != "" {
		var first struct {
			Content string `json:"content"`
		}
		var second struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(file), &first); err != nil {
			return nil, fmt.Errorf("decode direct workspace read: %w", err)
		}
		if err := json.Unmarshal([]byte(kb), &second); err != nil {
			return nil, fmt.Errorf("decode direct knowledge read: %w", err)
		}
		unique, greatest := make(map[string]bool), ""
		for _, data := range []string{first.Content, second.Text} {
			for line := range strings.SplitSeq(data, "\n") {
				id, tail, _ := strings.Cut(line, ",")
				kind, _, _ := strings.Cut(tail, ",")
				if kind == "P1" {
					unique[id] = true
					greatest = max(greatest, id)
				}
			}
		}
		p.results[0] = comparedReads{Expected: 2, Read: 2, Unique: len(unique), Greatest: greatest}
		p.dataBytes[0] = len(file) + len(kb)
		return nil, nil
	}
	if output := results["script"]; output != "" {
		encoded, _, _ := strings.Cut(output, "\n\nTool calls:")
		if err := json.Unmarshal([]byte(encoded), &p.results[1]); err != nil {
			return nil, fmt.Errorf("code summary: %s: %w", output, err)
		}
		p.dataBytes[1] = len(output)
		return nil, nil
	}
	if !slices.ContainsFunc(input.Tools, func(tool struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}) bool {
		return tool.Function.Name == p.readName
	}) {
		return []any{comparisonCall(0, "discover-reads", "search_tools", `{"query":"select:`+p.readName+`"}`)}, nil
	}
	if p.variant == 0 {
		return []any{
			comparisonCall(0, "file", "read", `{"path":"records.csv"}`),
			comparisonCall(1, "kb", p.readName, `{"id":"records","page":1}`),
		}, nil
	}
	source := fmt.Sprintf(`
const reads = await Promise.allSettled([
  tools.read({path: "records.csv"}),
  tools[%q]({id: "records", page: 1}),
]);
const ids = new Set();
let read = 0;
for (const [index, result] of reads.entries()) {
  if (result.status !== "fulfilled") continue;
  read += 1;
  const text = index === 0 ? result.value.content : result.value.text;
  for (const row of text.split("\n")) {
    const [id, category] = row.split(",");
    if (category === "P1") ids.add(id);
  }
}
return {expected: reads.length, read, unique: ids.size, greatest: [...ids].sort().at(-1)};
`, p.readName)
	args, err := json.Marshal(builtin.ScriptArguments{Summary: "Join workspace and archive", Script: source, Tools: []string{"read", p.readName}})
	if err != nil {
		return nil, err
	}
	return []any{comparisonCall(0, "script", string(builtin.RunScript), string(args))}, nil
}
