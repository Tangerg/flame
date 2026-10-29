package bootstrap

import (
	"bufio"
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func reviewRuntimeBackend(t *testing.T, fixture string) (string, func()) {
	t.Helper()
	directory := t.TempDir()
	executable := filepath.Join(directory, "reviews.test")
	build := exec.CommandContext(t.Context(), "go", "test", "-c", "-o", executable, ".")
	build.Dir = filepath.Join("..", "..", "..", "examples", "plugins", "reviews", "backend", "cmd", "reviews")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build review backend: %s, %v", output, err)
	}
	logPath := filepath.Join(directory, "backend.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	command := exec.CommandContext(t.Context(), executable, "-test.run=^"+fixture+"$", "-test.timeout=180s")
	command.Dir = directory
	command.Env = append(os.Environ(), "FLAME_TEST_REVIEWS_RUNTIME_DATA="+filepath.Join(directory, "private"))
	command.Stderr = log
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	type readiness struct {
		url string
		err error
	}
	ready := make(chan readiness, 1)
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(io.TeeReader(output, log))
		line, err := reader.ReadString('\n')
		ready <- readiness{strings.TrimSpace(line), err}
		_, readErr := io.Copy(io.Discard, reader)
		done <- errors.Join(readErr, command.Wait())
	}()
	closeBackend := sync.OnceFunc(func() {
		_ = input.Close()
		select {
		case err := <-done:
			if err != nil {
				output, _ := os.ReadFile(logPath)
				t.Errorf("review backend failed: %s, %v", output, err)
			}
		case <-time.After(lifecycleWaitBudget(t)):
			_ = command.Process.Kill()
			<-done
			t.Error("review backend did not finish its private-state assertions")
		}
	})
	t.Cleanup(closeBackend)
	select {
	case status := <-ready:
		if status.err != nil || !strings.HasPrefix(status.url, "http://127.0.0.1:") {
			t.Fatalf("review backend did not publish its endpoint: %s, %v", status.url, status.err)
		}
		return status.url, closeBackend
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
		return "", closeBackend
	}
}

func TestReviewReceiptHistoryMatchesModelToolAndExecutableCLI(t *testing.T) {
	for _, scenario := range []struct {
		name, input, outcome string
	}{
		{"missing_review", `{"id":"missing-history-review","expectedRevision":1,"status":"resolved"}`, `{"type":"notFound"}`},
		{"invalid_revision", `{"id":"example-review","expectedRevision":0,"status":"resolved"}`, `{"type":"invalidUpdate"}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			testReviewReceiptHistoryEntrances(t, scenario.input, scenario.outcome)
		})
	}
}

func testReviewReceiptHistoryEntrances(t *testing.T, updateInput, expected string) {
	t.Helper()
	url, closeBackend := reviewRuntimeBackend(t, "TestReceiptHistoryRuntimeFixture")
	defer closeBackend()
	source := reviewExamplePackage(t)
	descriptor, err := json.Marshal(map[string]any{
		"$schema":    "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{"reviews": testMCPServer(url)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), descriptor, 0600); err != nil {
		t.Fatal(err)
	}
	authorPluginPackage(t, source)
	client := reviewRefusalChat(t, updateInput)
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	cfg.modelProviders = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	defer runtime.close()
	installed := poMCPActivate(runtime, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: contribution.ID, Contract: contribution.Contract,
		Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		Method: "io.github.tangerg.flame.examples/reviews.list", Input: `{}`,
	}
	initial := runtime.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
	before, err := jsonvalue.Parse([]byte(initial.Output))
	if err != nil || initial.Failure != nil {
		t.Fatalf("initial review read: %+v, %v", initial, err)
	}
	key := "cli_" + strings.Repeat("d", 32)
	malformed := request
	malformed.Method = "io.github.tangerg.flame.examples/reviews.update"
	malformed.Input = `{"id":"` + strings.Repeat("x", 129) + `","expectedRevision":1,"status":"resolved"}`
	refused := runtime.endpoint.Invoke(t.Context(), delivery.PluginsCall, malformed, delivery.Options{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	if refused.Failure == nil || !errors.Is(refused.Failure, protocol.ErrInvalidParams) || refused.Value != nil {
		t.Fatalf("malformed input entered command execution: %+v", refused)
	}
	assertReviewToolRefusal(t, runtime, expected)
	want, err := jsonvalue.Parse([]byte(expected))
	if err != nil {
		t.Fatal(err)
	}
	runCLI, closeCLI := reviewCLICommand(t, runtime)
	defer closeCLI()
	request.Method, request.Input = "io.github.tangerg.flame.examples/reviews.update", updateInput
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	arguments := []string{"plugins", "call", "--request", string(encoded), "--command-id", key}
	assertReply := func(reply *protocol.CallPluginResponse) {
		t.Helper()
		actual, err := jsonvalue.Parse([]byte(reply.Output))
		if err != nil || reply.Failure != nil || actual != want {
			t.Fatalf("business refusal lost original facts: %+v, want %s, %v", reply, want.JSON(), err)
		}
	}
	for range 2 {
		output := runCLI(arguments...)
		var reply protocol.CallPluginResponse
		if err := json.Unmarshal(output, &reply, json.RejectUnknownMembers(true)); err != nil {
			t.Fatalf("CLI lost the structured history result: %s, %v", output, err)
		}
		assertReply(&reply)
	}
	assertReply(runtime.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse))
	request.Method, request.Input = "io.github.tangerg.flame.examples/reviews.list", `{}`
	current := runtime.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
	after, err := jsonvalue.Parse([]byte(current.Output))
	if err != nil || current.Failure != nil || after != before {
		t.Fatalf("refused entrances changed the private review: %+v, %v", current, err)
	}
}

func reviewRefusalChat(t *testing.T, updateInput string) chatclient.Client {
	t.Helper()
	const toolName = "reviews_update_review"
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("business refusal"), nil
		}
		conversation := withoutDeferredCatalog(request)
		for _, part := range conversation[len(conversation)-1].Parts {
			if part.ToolResult != nil && part.ToolResult.Name == toolName {
				return completedTextResponse("observed the business refusal"), nil
			}
		}
		call := chat.ToolCall{ID: "history-call", Name: "search_tools", Arguments: `{"query":"select:` + toolName + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == toolName {
				call.Name, call.Arguments = toolName, updateInput
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertReviewToolRefusal(t *testing.T, runtime *poMCPRuntime, expected string) {
	t.Helper()
	const toolName = "reviews_update_review"
	session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Receipt history"}, "session").(*protocol.Session)
	started := runtime.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Resolve the inspected review"}}}, delivery.Options{IdempotencyKey: "history-start", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	events := iter.Seq2[protocol.RunEvent, error](func(yield func(protocol.RunEvent, error) bool) {
		for event, err := range started.Events {
			if err != nil {
				t.Errorf("history Run event: %v", err)
				return
			}
			if !yield(event.(protocol.RunEvent), nil) {
				return
			}
		}
	})
	waitForRunEvents(t, collectRunEvents(events), "review history")
	runID := started.Value.(*protocol.StartRunResponse).RunID
	finished := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("known history refusal became uncertain execution: %+v", finished)
	}
	items := runtime.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID}}, "").(*protocol.ListItemsResponse)
	want, err := jsonvalue.Parse([]byte(expected))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, item := range items.Data {
		if item.Tool == nil || item.Tool.Name != toolName {
			continue
		}
		calls++
		actual, err := jsonvalue.Encode(item.Tool.Result)
		if err != nil || actual != want || item.Status != protocol.ItemStatusIncomplete || item.Error == nil || item.Error.Type != protocol.ProblemToolFailed {
			t.Fatalf("Tool lost its history facts/classification: %+v, %v", item, err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected one original history-refused Tool, got %d", calls)
	}
}
