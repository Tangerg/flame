package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestReviewBackendCommittedResponseLossPreservesUnknownRunAndReceipt(t *testing.T) {
	source := reviewExamplePackage(t)
	evidencePath := filepath.Join(t.TempDir(), "lost-response.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "response-loss"), "./testdata/review-response-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build transport probe: %s, %v", output, err)
	}
	manifest, err := json.Marshal(map[string]any{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{"reviews": map[string]any{
			"type": "stdio", "command": "./bin/response-loss", "args": []string{evidencePath, "./bin/reviews"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	var modelCalls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("review"), nil
		}
		modelCalls.Add(1)
		call := chat.ToolCall{ID: "provider-reused", Name: "search_tools", Arguments: `{"query":"select:reviews_update_review"}`}
		for _, definition := range request.Tools {
			if definition.Name == "reviews_update_review" {
				call.Name, call.Arguments = definition.Name, `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
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
	cfg.ChatResolver = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	installed := poMCPActivate(runtime, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 2 })
	session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Lost review response"}, "session").(*protocol.Session)
	invoke := func(method delivery.Name, request any, key string) delivery.Result {
		t.Helper()
		result := runtime.endpoint.Invoke(t.Context(), method, request, delivery.Options{
			IdempotencyKey: key,
			RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion, ClientCapabilities: &protocol.ClientCapabilities{
				InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval},
			}},
		})
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
		waitForRunEvents(t, collectRunEvents(events), "lost review response")
	}
	started := invoke(delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Resolve the inspected review"}}}, "start")
	drain(started)
	runID := started.Value.(*protocol.StartRunResponse).RunID
	pending := invoke(delivery.InterruptsList, protocol.ListInterruptsRequest{RootRunID: runID}, "").Value.(*protocol.Page[protocol.PendingInterruptSet])
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 || pending.Data[0].Interrupts[0].Type != protocol.InterruptApproval {
		t.Fatalf("approval = %+v", pending.Data)
	}
	request := protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{ItemID: pending.Data[0].Interrupts[0].ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove}}}}
	accepted := invoke(delivery.RunsResume, request, "approved")
	drain(accepted)
	finished := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
	if finished.Status != protocol.RunStatusFinished || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeLost || len(finished.Outcome.UnresolvedEffects) != 1 {
		t.Fatalf("lost backend response became a known outcome: %+v", finished)
	}
	items := runtime.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID}}, "").(*protocol.ListItemsResponse)
	var updateCalls int
	for _, item := range items.Data {
		if item.Tool != nil && item.Tool.Name == "reviews_update_review" {
			updateCalls++
			if item.Tool.Result != nil || item.Status == protocol.ItemStatusCompleted {
				t.Fatalf("unobserved backend result became completed Tool content: %+v", item)
			}
		}
	}
	if updateCalls != 1 || modelCalls.Load() != 2 {
		t.Fatalf("uncertain update was replaced: update calls=%d, model calls=%d", updateCalls, modelCalls.Load())
	}
	replayed := invoke(delivery.RunsResume, request, "approved")
	if !reflect.DeepEqual(accepted.Value, replayed.Value) || modelCalls.Load() != 2 {
		t.Fatal("command replay replaced uncertain execution")
	}
	runtime.close()

	encoded, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		DataDirectory string `json:"dataDirectory"`
		Request       struct {
			Params struct {
				Meta      sdk.Meta       `json:"_meta"`
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		} `json:"request"`
		Response struct {
			Result sdk.CallToolResult `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatal(err)
	}
	identity, _ := evidence.Request.Params.Meta["io.github.tangerg.flame/invocationId"].(string)
	if identity == "" || identity == "provider-reused" || evidence.Response.Result.IsError || evidence.Response.Result.StructuredContent == nil {
		t.Fatalf("probe did not witness the canonical committed invocation: %s", encoded)
	}
	dataRoot, err := filepath.EvalSymlinks(cfg.Stores.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	backendData, err := filepath.EvalSymlinks(evidence.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(dataRoot, backendData)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		t.Fatal("backend data escaped the isolated Runtime")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(source, "bin", "reviews"))
	command.Env = append(os.Environ(), "PLUGIN_DATA="+evidence.DataDirectory)
	mcpClient := sdk.NewClient(&sdk.Implementation{Name: "receipt-recovery-test", Version: "1"}, nil)
	backend, err := mcpClient.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Error(err)
		}
	})
	result, err := backend.CallTool(ctx, &sdk.CallToolParams{Name: evidence.Request.Params.Name, Arguments: evidence.Request.Params.Arguments, Meta: evidence.Request.Params.Meta})
	if err != nil || result.IsError || !reflect.DeepEqual(result.StructuredContent, evidence.Response.Result.StructuredContent) {
		t.Fatalf("original invocation did not recover its committed receipt: %+v, %v", result, err)
	}
	stale, err := backend.CallTool(ctx, &sdk.CallToolParams{Name: evidence.Request.Params.Name, Arguments: evidence.Request.Params.Arguments, Meta: sdk.Meta{"io.github.tangerg.flame/invocationId": "new-inspected-call"}})
	if err != nil || !stale.IsError {
		t.Fatalf("a replacement call bypassed the original revision: %+v, %v", stale, err)
	}
	encoded, err = json.Marshal(stale.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var conflict struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(encoded, &conflict); err != nil || conflict.Type != "revisionConflict" {
		t.Fatalf("replacement call did not report the revision conflict: %s, %v", encoded, err)
	}
	listed, err := backend.CallTool(ctx, &sdk.CallToolParams{Name: "list_reviews", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("current backend read: %+v, %v", listed, err)
	}
	encoded, err = json.Marshal(listed.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var current struct {
		Reviews []struct {
			Revision int64  `json:"revision"`
			Status   string `json:"status"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(encoded, &current); err != nil || len(current.Reviews) != 1 || current.Reviews[0].Revision != 2 || current.Reviews[0].Status != "resolved" {
		t.Fatalf("recovery advanced the review again: %s, %v", encoded, err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	runtime = poMCPOpen(t, cfg)
	restored := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
	if !reflect.DeepEqual(restored.Outcome, finished.Outcome) || modelCalls.Load() != 2 {
		t.Fatalf("restart invented settlement of an unobserved effect: %+v", restored)
	}
}
