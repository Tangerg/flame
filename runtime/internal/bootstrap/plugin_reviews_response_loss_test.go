package bootstrap

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestReviewCommandWithoutRunRecoversItsOriginalOwnerAcrossReplacementAndRestart(t *testing.T) {
	source := reviewExamplePackage(t)
	method := "io.github.tangerg.flame.examples/reviews.update"
	evidence := filepath.Join(t.TempDir(), "committed.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "response-loss"), "./testdata/managed-message-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build response-loss fixture: %s, %v", output, err)
	}
	manifest, err := json.Marshal(map[string]any{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{"reviews": map[string]any{
			"type": "stdio", "command": "./bin/response-loss", "args": []string{evidence, "./bin/reviews", method, "", "response"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	authorPluginPackage(t, source)
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: contribution.ID, Contract: contribution.Contract, Method: method,
		Target: portable.Target{Scope: "installation", Reference: installed.ID},
		Input:  `{"id":"example-review","expectedRevision":1,"status":"resolved"}`,
	}
	options := delivery.Options{IdempotencyKey: "original-review-command", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options); result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("lost response became a definite result: %+v", result)
	}
	encoded, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatalf("backend did not commit before response loss: %v", err)
	}
	var captured struct {
		Response struct {
			Result jsontext.Value `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(encoded, &captured); err != nil {
		t.Fatal(err)
	}
	committed, err := qimumcp.DecodeReply[jsontext.Value](captured.Response.Result)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := jsonvalue.Parse(committed)
	if err != nil {
		t.Fatal(err)
	}
	// A redispatched mutation would recreate this evidence and lose its reply,
	// so backend idempotency cannot hide an incorrect host replay.
	if err := os.Remove(evidence); err != nil {
		t.Fatal(err)
	}
	commandID := operation.Identity(operation.Bind(t.Context(), cfg.stores.IdempotencyNamespace.String(), options.IdempotencyKey))
	original, found, err := cfg.stores.PluginCommands.Get(t.Context(), commandID)
	if err != nil || !found || original.Snapshot().Phase != plugin.CommandDispatched || original.Snapshot().Invocation.Origin != extension.Human {
		t.Fatalf("human command lost its pending owner: %+v, %t, %v", original, found, err)
	}
	installed = stageCommandReplacement(t, r, installed, source)
	assertUnresolvedInstallationIsHeld(t, r, installed)
	replacement := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: reviewExamplePackage(t)}, "install-replacement").(*protocol.PluginInstallation)
	replacement = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: replacement.ID, Enabled: true}, "enable-replacement").(*protocol.PluginInstallation)
	replacementServer := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: replacement.ID}, Name: "reviews"}
	r.poMCPTools(&replacementServer, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-original")
	r = reopenPluginRuntime(t, r, &cfg)
	assertUnresolvedInstallationIsHeld(t, r, installed)
	restored, found, err := cfg.stores.PluginCommands.Get(t.Context(), commandID)
	if err != nil || !found || !reflect.DeepEqual(restored.Snapshot(), original.Snapshot()) {
		t.Fatalf("restart changed the original pending binding: %+v, %t, %v", restored, found, err)
	}
	lookup := protocol.PluginCommandRequest{Namespace: cfg.stores.IdempotencyNamespace.String(), Key: options.IdempotencyKey}
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsRetireCommand, lookup, delivery.Options{}); !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("dispatched command became eligible for retirement: %+v", result)
	}
	assertUnresolvedInstallationIsHeld(t, r, installed)
	routed := r.must(delivery.PluginsGetCommand, lookup, "").(*protocol.CallPluginRequest)
	admittedRequest := request
	admittedRequest.Input = string(original.Snapshot().Input.JSON())
	if !reflect.DeepEqual(*routed, admittedRequest) {
		t.Fatalf("public restart lookup changed original request: %+v", routed)
	}
	otherStore := lookup
	otherStore.Namespace = "idp_00000000000000000000000000000000"
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsGetCommand, otherStore, delivery.Options{}); !errors.Is(result.Failure, protocol.ErrIdempotencyStoreMismatch) {
		t.Fatalf("public route lookup crossed store identity: %+v", result)
	}
	if _, err := os.Stat(evidence); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("route read dispatched backend work: %v", err)
	}
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options); result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("disabled owner settled an unconfirmed command: %+v", result)
	}
	retargeted := request
	retargeted.InstallationID, retargeted.Digest, retargeted.Target.Reference = replacement.ID, replacement.Selected.Digest, replacement.ID
	if result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, retargeted, options); !errors.Is(result.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("original identity adopted a replacement target: %+v", result)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable-original")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	recovered := r.must(delivery.PluginsCall, request, options.IdempotencyKey).(*protocol.CallPluginResponse)
	actual, err := jsonvalue.Parse([]byte(recovered.Output))
	if err != nil || recovered.Failure != nil || actual != expected {
		t.Fatalf("receipt differs from the committed private result: %+v, %v", recovered, err)
	}
	if _, err := os.Stat(evidence); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("receipt recovery redispatched the mutation: %v", err)
	}
	observed, found, err := cfg.stores.PluginCommands.Get(t.Context(), commandID)
	if err != nil || !found || observed.Snapshot().Phase != plugin.CommandObserved {
		t.Fatalf("original receipt did not settle its command: %+v, %t, %v", observed, found, err)
	}
	read := func(owner *protocol.PluginInstallation, revision int64, status string) {
		t.Helper()
		query := request
		query.InstallationID, query.Digest, query.Target.Reference = owner.ID, owner.Selected.Digest, owner.ID
		query.Method, query.Input = "io.github.tangerg.flame.examples/reviews.list", `{}`
		reply := r.must(delivery.PluginsCall, query, "").(*protocol.CallPluginResponse)
		var page struct {
			Reviews []struct {
				Revision int64  `json:"revision"`
				Status   string `json:"status"`
			} `json:"reviews"`
		}
		if err := json.Unmarshal([]byte(reply.Output), &page); err != nil || reply.Failure != nil || len(page.Reviews) != 1 || page.Reviews[0].Revision != revision || page.Reviews[0].Status != status {
			t.Fatalf("private owner advanced unexpectedly: %+v, %v", reply, err)
		}
	}
	read(installed, 2, "resolved")
	r.poMCPTools(&replacementServer, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	read(replacement, 1, "open")
	runs := r.must(delivery.RunsList, protocol.ListRunsRequest{}, "").(*protocol.Page[protocol.RunRef])
	if len(runs.Data) != 0 {
		t.Fatalf("human recovery manufactured a Run: %+v", runs.Data)
	}
	r.must(delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Staged.Digest}, "select-after-receipt")
}

func TestReviewCapabilityCommittedResponseLossRestoresOriginalExportWithoutResending(t *testing.T) {
	source := reviewExamplePackage(t)
	probe := filepath.Join(source, "bin", "response-loss")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", probe, "./testdata/managed-message-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build transport probe: %s, %v", output, err)
	}
	data, err := os.ReadFile(filepath.Join(source, "io.github.tangerg.flame", "contracts", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.ParseCapability(data)
	if err != nil {
		t.Fatal(err)
	}
	var method string
	for _, descriptor := range contract.Descriptor().Methods {
		if descriptor.Kind == portable.Mutation {
			if method != "" {
				t.Fatal("fixture requires one mutation")
			}
			method = descriptor.Name
		}
	}
	if method == "" {
		t.Fatal("fixture has no mutation")
	}
	privateData := t.TempDir()
	evidence := filepath.Join(t.TempDir(), "committed.json")
	guard := filepath.Join(t.TempDir(), "unexpected-mutation.json")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	connect := func(path, stage string) *sdk.ClientSession {
		t.Helper()
		command := exec.CommandContext(ctx, probe, path, filepath.Join(source, "bin", "reviews"), method, "", stage)
		command.Env = append(os.Environ(), "PLUGIN_DATA="+privateData)
		client := sdk.NewClient(&sdk.Implementation{Name: "capability-recovery-test"}, nil)
		if err := qimumcp.RegisterClient(client); err != nil {
			t.Fatal(err)
		}
		session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	session := connect(evidence, "response")
	bound, err := qimumcp.ResolveCapability(ctx, session, "reviews", contract)
	if err != nil {
		t.Fatal(err)
	}
	write, err := bound.Method(method)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Parse([]byte(`{"id":"example-review","expectedRevision":1,"status":"resolved"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.Invoke(ctx, "original-command", input); err == nil {
		t.Fatal("lost acknowledgement became known success")
	} else if _, known := errors.AsType[*portable.Refusal](err); known {
		t.Fatalf("lost observation became a business refusal: %v", err)
	}
	_ = session.Close()
	encoded, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Response struct {
			Result jsontext.Value `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(encoded, &captured); err != nil {
		t.Fatal(err)
	}
	accepted, err := qimumcp.DecodeReply[jsontext.Value](captured.Response.Result)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := jsonvalue.Parse(accepted)
	if err != nil {
		t.Fatal(err)
	}
	var outcome struct {
		Type   string `json:"type"`
		Review struct {
			Revision int64 `json:"revision"`
		} `json:"review"`
	}
	if err := json.Unmarshal(accepted, &outcome); err != nil || outcome.Type != "updated" || outcome.Review.Revision != 2 {
		t.Fatalf("probe did not witness committed business evidence: %s, %v", accepted, err)
	}
	restarted := connect(guard, "request")
	original, err := qimumcp.NewCapabilityBinding(restarted, "reviews", contract)
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := original.Method(method)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, found, err := lookup.ReadReceipt(ctx, "original-command", input); err != nil || !found || recovered != expected {
		t.Fatalf("original export recovery = %s, %t, %v", recovered, found, err)
	}
	if _, found, err := lookup.ReadReceipt(ctx, "never-admitted", input); err != nil || found {
		t.Fatalf("missing receipt became success: %t, %v", found, err)
	}
	if _, err := os.Stat(guard); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery attempted to resend the mutation: %v", err)
	}
}

func TestReviewBackendCommittedResponseLossPreservesUnknownRunAndReceipt(t *testing.T) {
	source := reviewExamplePackage(t)
	evidencePath := filepath.Join(t.TempDir(), "lost-response.json")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin", "response-loss"), "./testdata/managed-message-loss")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build transport probe: %s, %v", output, err)
	}
	manifest, err := json.Marshal(map[string]any{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{"reviews": map[string]any{
			"type": "stdio", "command": "./bin/response-loss", "args": []string{evidencePath, "./bin/reviews", "tools/call", "update_review", "response"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	authorPluginPackage(t, source)
	metadata, err := os.ReadFile(filepath.Join(source, "io.github.tangerg.flame", "contracts", "methods.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	var methods []struct {
		Name string              `json:"name"`
		Kind portable.MethodKind `json:"kind"`
	}
	if err := json.Unmarshal(metadata, &methods); err != nil {
		t.Fatal(err)
	}
	var updateMethod string
	for _, method := range methods {
		if method.Kind == portable.Mutation {
			if updateMethod != "" {
				t.Fatal("receipt fixture requires one declared mutation")
			}
			updateMethod = method.Name
		}
	}
	if updateMethod == "" {
		t.Fatal("backend contract does not declare its mutation")
	}
	capabilityData, err := os.ReadFile(filepath.Join(source, "io.github.tangerg.flame", "contracts", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.ParseCapability(capabilityData)
	if err != nil {
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
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.chatResolver = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	installed := poMCPActivate(runtime, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Lost review response"}, "session").(*protocol.Session)
	invoke := func(method delivery.Name, request any, key string) delivery.Result {
		t.Helper()
		result := runtime.endpoint.Invoke(t.Context(), method, request, delivery.Options{
			IdempotencyKey: key,
			RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientCapabilities: &protocol.ClientCapabilities{
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
	if err := cfg.stores.Close(); err != nil {
		t.Fatal(err)
	}

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
				Arguments jsontext.Value `json:"arguments"`
			} `json:"params"`
		} `json:"request"`
		Response struct {
			Result sdk.CallToolResult `json:"result"`
		} `json:"response"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatal(err)
	}
	var invocation portable.Invocation
	if err := json.Unmarshal(evidence.Request.Params.Meta[qimumcp.InvocationMetadataKey], &invocation); err != nil {
		t.Fatal(err)
	}
	identity := invocation.ID
	if invocation.Origin != extension.Model || invocation.Parent == "" || invocation.Target.Scope != "installation" {
		t.Fatalf("calling-owner evidence was lost: %+v", invocation)
	}
	if identity == "" || identity == "provider-reused" || evidence.Response.Result.IsError || evidence.Response.Result.StructuredContent == nil {
		t.Fatalf("probe did not witness the canonical committed invocation: %s", encoded)
	}
	dataRoot, err := filepath.EvalSymlinks(cfg.stores.DataDirectory)
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
	if err := qimumcp.RegisterClient(mcpClient); err != nil {
		t.Fatal(err)
	}
	backend, err := mcpClient.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Error(err)
		}
	})
	bound, err := qimumcp.NewCapabilityBinding(backend, "reviews", contract)
	if err != nil {
		t.Fatal(err)
	}
	original, err := bound.Method(updateMethod)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Parse(evidence.Request.Params.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	recovered, found, err := original.ReadReceipt(ctx, identity, input)
	if err != nil || !found {
		t.Fatalf("read-only lookup did not recover the committed receipt: %+v, %t, %v", recovered, found, err)
	}
	result := jsontext.Value(recovered.JSON())
	expected, err := json.Marshal(evidence.Response.Result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	expectedResult := jsontext.Value(expected)
	if err := expectedResult.Format(jsontext.ReorderRawObjects(true)); err != nil {
		t.Fatal(err)
	}
	if err := result.Format(jsontext.ReorderRawObjects(true)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, expectedResult) {
		t.Fatalf("receipt changed the original business evidence: %s, want %s", result, expectedResult)
	}
	if _, found, err := original.ReadReceipt(ctx, "never-dispatched", input); err != nil || found {
		t.Fatalf("absent receipt became mutation evidence: %t, %v", found, err)
	}
	var inspectedMetadata sdk.Meta
	if err := inspectedMetadata.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: "new-inspected-call", Origin: extension.Model, Parent: invocation.Parent, Target: invocation.Target}); err != nil {
		t.Fatal(err)
	}
	stale, err := backend.CallTool(ctx, &sdk.CallToolParams{Name: evidence.Request.Params.Name, Arguments: evidence.Request.Params.Arguments, Meta: inspectedMetadata})
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
	cfg.stores, err = persistence.Open(t.Context(), persistence.Config{DataDirectory: cfg.stores.DataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cfg.stores.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg.providerRegistry = cfg.stores.Providers
	runtime = poMCPOpen(t, cfg)
	restored := runtime.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
	if !reflect.DeepEqual(restored.Outcome, finished.Outcome) || modelCalls.Load() != 2 {
		t.Fatalf("restart invented settlement of an unobserved effect: %+v", restored)
	}
}
