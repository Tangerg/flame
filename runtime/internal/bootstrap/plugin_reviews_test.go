package bootstrap

import (
	"bytes"
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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func reviewExamplePackage(t *testing.T) string {
	return reviewAuthorPackage(t, "package", "reviews")
}

func reviewAuthorPackage(t *testing.T, packagePath, executable string) string {
	t.Helper()
	source := t.TempDir()
	authorRoot := filepath.Join("..", "..", "..", "examples", "plugins", "reviews")
	packageRoot := filepath.Join(authorRoot, packagePath)
	if err := os.CopyFS(source, os.DirFS(packageRoot)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", executable), "./cmd/"+executable)
	command.Dir = filepath.Join(authorRoot, "backend")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build review backend: %s, %v", output, err)
	}
	return source
}

func reviewCLICommand(t *testing.T, runtime *poMCPRuntime) (func(...string) []byte, func()) {
	t.Helper()
	invoke, closeCLI := reviewCLIInvocation(t, runtime)
	return func(arguments ...string) []byte {
		t.Helper()
		output, stderr, err := invoke(arguments...)
		if err != nil {
			t.Fatalf("CLI %s: %s, %s, %v", strings.Join(arguments[:min(2, len(arguments))], " "), output, stderr, err)
		}
		return output
	}, closeCLI
}

func reviewCLIInvocation(t *testing.T, runtime *poMCPRuntime) (func(...string) ([]byte, []byte, error), func()) {
	t.Helper()
	directory := t.TempDir()
	executable := filepath.Join(directory, "flame")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", executable, ".")
	build.Dir = filepath.Join("..", "..", "..", "apps", "cli")
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
	remote := httptest.NewServer(server.Handler())
	closeCLI := sync.OnceFunc(func() {
		remote.Close()
		_ = server.Close()
	})
	t.Cleanup(closeCLI)
	config := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(config, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runCLI := func(arguments ...string) ([]byte, []byte, error) {
		t.Helper()
		command := exec.CommandContext(t.Context(), executable, append([]string{"--config", config, "--runtime-url", remote.URL}, arguments...)...)
		command.Dir = directory
		command.Env = append(os.Environ(), "FLAME_HOME="+filepath.Join(directory, "home"), "FLAME_RUNTIME_TOKEN=review-acceptance-token")
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		return output, stderr.Bytes(), err
	}
	return runCLI, closeCLI
}

func assertReviewCLIEntrances(t *testing.T, runtime *poMCPRuntime, installed *protocol.PluginInstallation, sessionID, runID string, expected, conflict any) {
	t.Helper()
	runCLI, closeCLI := reviewCLICommand(t, runtime)
	defer closeCLI()
	output := runCLI("sessions", "show", sessionID, "--json")
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
	found := false
	for _, block := range snapshot.Transcript {
		if block.RunID == runID && block.Tool != nil && block.Tool.Name == "reviews_list_reviews" {
			if !reflect.DeepEqual(block.Tool.Result, result) {
				t.Fatalf("CLI result = %+v, canonical result = %+v", block.Tool.Result, result)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("CLI snapshot omitted the canonical backend read")
	}
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: contribution.ID, Contract: contribution.Contract,
		Target: portable.Target{Scope: "installation", Reference: installed.ID},
		Method: "io.github.tangerg.flame.examples/reviews.list", Input: `{}`,
	}
	call := func(key string, expected any) {
		t.Helper()
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		arguments := []string{"plugins", "call", "--request", string(encoded)}
		if key != "" {
			arguments = append(arguments, "--command-id", key)
		}
		output := runCLI(arguments...)
		var reply protocol.CallPluginResponse
		if err := json.Unmarshal(output, &reply, json.RejectUnknownMembers(true)); err != nil || reply.Failure != nil {
			t.Fatalf("CLI lost the structured business result: %s, %v", output, err)
		}
		want, err := jsonvalue.Encode(expected)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := jsonvalue.Parse([]byte(reply.Output))
		if err != nil || actual != want {
			t.Fatalf("CLI operation differs from the Tool owner's result: %s, want %s, %v", reply.Output, want.JSON(), err)
		}
	}
	call("", expected)
	request.Method, request.Input = "io.github.tangerg.flame.examples/reviews.update", `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
	key := "cli_" + strings.Repeat("c", 32)
	call(key, conflict)
	call(key, conflict)
	request.Method, request.Input = "io.github.tangerg.flame.examples/reviews.list", `{}`
	call("", expected)
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
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.chatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	source := reviewExamplePackage(t)
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Servers) != 1 {
		t.Fatalf("example admission = %+v", installed.Selected)
	}
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Review example"}, "session").(*protocol.Session)
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
	r = reopenPluginRuntime(t, r, &cfg)
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
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
	phase.Store(4)
	stale := start()
	drain(invoke(delivery.RunsResume, approval(stale), "stale-approved"))
	item = toolResult(stale, "reviews_update_review")
	encoded, err = json.Marshal(item.Tool.Result)
	if err != nil || item.Status != protocol.ItemStatusIncomplete || !strings.Contains(string(encoded), `"type":"revisionConflict"`) {
		t.Fatalf("stale update = %+v, %s, %v", item, encoded, err)
	}
	if item.Error == nil || item.Error.Type != protocol.ProblemToolFailed {
		t.Fatalf("known revision refusal lost its Tool failure classification: %+v", item.Error)
	}
	finished := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: stale}, "").(*protocol.RunRef)
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("known revision refusal became uncertain execution: %+v", finished)
	}
	assertReviewCLIEntrances(t, r, installed, session.ID, read, toolResult(read, "reviews_list_reviews").Tool.Result, item.Tool.Result)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	if current := toolResult(update, "reviews_update_review"); !reflect.DeepEqual(current.Tool.Result, savedResult) {
		t.Fatal("withdrawal changed history")
	}
	r = reopenPluginRuntime(t, r, &cfg)
	if current := toolResult(update, "reviews_update_review"); current.Status != protocol.ItemStatusCompleted {
		t.Fatalf("withdrawn backend lost history: %+v", current)
	}
}
