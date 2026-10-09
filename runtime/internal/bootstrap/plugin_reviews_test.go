package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"iter"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func reviewExamplePackage(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	packageRoot := filepath.Join("..", "..", "..", "examples", "plugins", "reviews", "package")
	if err := os.CopyFS(source, os.DirFS(packageRoot)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "reviews"), "./cmd/reviews")
	command.Dir = filepath.Join(packageRoot, "..", "backend")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build review backend: %s, %v", output, err)
	}
	return source
}

func assertReviewCLIProjection(t *testing.T, runtime *poMCPRuntime, sessionID, runID string, expected any) {
	t.Helper()
	directory := t.TempDir()
	executable := filepath.Join(directory, "flame")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", executable, ".")
	build.Dir = filepath.Join("..", "..", "..", "cli")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %s, %v", output, err)
	}
	discovery := runtime.must(delivery.RuntimeDiscover, struct{}{}, "").(*protocol.DiscoverResponse)
	server, err := flamehttp.NewServer(flamehttp.Config{
		Endpoint: runtime.endpoint, Addr: "127.0.0.1:0", ServerInfo: discovery.ServerInfo, LocalToken: "review-acceptance-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	remote := httptest.NewServer(server.Handler())
	defer remote.Close()
	config := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(config, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), executable, "--config", config, "--runtime-url", remote.URL, "sessions", "show", sessionID, "--json")
	command.Dir = directory
	command.Env = append(os.Environ(), "FLAME_HOME="+filepath.Join(directory, "home"), "FLAME_RUNTIME_TOKEN=review-acceptance-token")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI read: %s, %v", output, err)
	}
	var snapshot struct {
		Transcript []struct {
			RunID string `json:"runId"`
			Tool  *struct {
				Name   string `json:"name"`
				Result any    `json:"result"`
			} `json:"tool"`
		} `json:"transcript"`
	}
	if err := json.Unmarshal(output, &snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	var result any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	for _, block := range snapshot.Transcript {
		if block.RunID == runID && block.Tool != nil && block.Tool.Name == "reviews_list_reviews" {
			if !reflect.DeepEqual(block.Tool.Result, result) {
				t.Fatalf("CLI result = %+v, canonical result = %+v", block.Tool.Result, result)
			}
			return
		}
	}
	t.Fatal("CLI snapshot omitted the canonical backend read")
}

func TestReviewBackendUsesInstallationRunApprovalAndRecoveryOwners(t *testing.T) {
	cfg := poMCPConfig(t)
	var phase atomic.Int32
	phase.Store(1)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("review"), nil
		}
		name, arguments := "reviews_list_reviews", `{}`
		if phase.Load() == 2 || phase.Load() == 4 {
			name, arguments = "reviews_update_review", `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
		}
		conversation := withoutDeferredCatalog(request)
		for _, part := range conversation[len(conversation)-1].Parts {
			if part.ToolResult != nil && part.ToolResult.Name == name {
				return completedTextResponse("observed"), nil
			}
		}
		call := chat.ToolCall{ID: "provider-reused", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call.Name, call.Arguments = name, arguments
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	source := reviewExamplePackage(t)
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Servers) != 1 {
		t.Fatalf("example admission = %+v", installed.Selected)
	}
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 2 })
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Review example"}, "session").(*protocol.Session)
	options := func(key string) delivery.Options {
		return delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}}}}
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
		waitForRunEvents(t, collectRunEvents(events), "review phase")
	}
	start := func() string {
		t.Helper()
		result := invoke(delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: fmt.Sprintf("Review phase %d", phase.Load())}}}, fmt.Sprintf("start-%d", phase.Load()))
		drain(result)
		return result.Value.(*protocol.StartRunResponse).RunID
	}
	approval := func(runID string) protocol.ResumeRunRequest {
		t.Helper()
		pending := invoke(delivery.InterruptsList, protocol.ListInterruptsRequest{RootRunID: runID}, "").Value.(*protocol.Page[protocol.PendingInterruptSet])
		if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 || pending.Data[0].Interrupts[0].Type != protocol.InterruptApproval {
			t.Fatalf("approval = %+v", pending.Data)
		}
		return protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{ItemID: pending.Data[0].Interrupts[0].ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove}}}}
	}
	toolResult := func(runID, name string) protocol.Item {
		t.Helper()
		items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID}}, "").(*protocol.ListItemsResponse)
		for _, item := range items.Data {
			if item.Tool != nil && item.Tool.Name == name && item.Tool.Result != nil {
				return item
			}
		}
		t.Fatalf("missing %s result: %+v", name, items.Data)
		return protocol.Item{}
	}
	assertList := func(runID string, revision int64, status string) {
		t.Helper()
		item := toolResult(runID, "reviews_list_reviews")
		encoded, err := json.Marshal(item.Tool.Result)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Reviews []struct {
				Revision int64  `json:"revision"`
				Status   string `json:"status"`
			} `json:"reviews"`
		}
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Reviews) != 1 || result.Reviews[0].Revision != revision || result.Reviews[0].Status != status {
			t.Fatalf("backend read = %s", encoded)
		}
	}

	read := start()
	drain(invoke(delivery.RunsResume, approval(read), "read-approved"))
	assertList(read, 1, "open")
	phase.Store(2)
	update := start()
	request := approval(update)
	// The actual backend process closes while its update is still awaiting the
	// existing Interrupt. Its private data and the Runtime checkpoint survive.
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 2 })
	accepted := invoke(delivery.RunsResume, request, "update-approved")
	drain(accepted)
	item := toolResult(update, "reviews_update_review")
	if item.Status != protocol.ItemStatusCompleted {
		t.Fatalf("update = %+v", item)
	}
	savedResult := item.Tool.Result
	encoded, err := json.Marshal(item.Tool.Result)
	if err != nil || !strings.Contains(string(encoded), `"revision":2`) || !strings.Contains(string(encoded), `"type":"updated"`) {
		t.Fatalf("canonical update = %s, %v", encoded, err)
	}
	replayed := invoke(delivery.RunsResume, request, "update-approved")
	if !reflect.DeepEqual(accepted.Value, replayed.Value) {
		t.Fatal("lost acknowledgement replaced the accepted Run")
	}
	phase.Store(3)
	read = start()
	drain(invoke(delivery.RunsResume, approval(read), "read-after-update"))
	assertList(read, 2, "resolved")
	assertReviewCLIProjection(t, r, session.ID, read, toolResult(read, "reviews_list_reviews").Tool.Result)
	phase.Store(4)
	stale := start()
	drain(invoke(delivery.RunsResume, approval(stale), "stale-approved"))
	item = toolResult(stale, "reviews_update_review")
	encoded, err = json.Marshal(item.Tool.Result)
	if err != nil || item.Status != protocol.ItemStatusIncomplete || !strings.Contains(string(encoded), `"type":"revisionConflict"`) {
		t.Fatalf("stale update = %+v, %s, %v", item, encoded, err)
	}
	r.must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke")
	if current := toolResult(update, "reviews_update_review"); !reflect.DeepEqual(current.Tool.Result, savedResult) {
		t.Fatal("withdrawal changed history")
	}
	r.close()
	r = poMCPOpen(t, cfg)
	if current := toolResult(update, "reviews_update_review"); current.Status != protocol.ItemStatusCompleted {
		t.Fatalf("withdrawn backend lost history: %+v", current)
	}
}
