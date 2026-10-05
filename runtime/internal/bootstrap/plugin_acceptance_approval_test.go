package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

// P-O03: a standing approval binds the admitted release through the source
// fingerprint, so selecting another release digest makes it stale and the next
// call prompts again even though the server declaration is unchanged.
func TestPluginAcceptanceApprovalStaleAfterReleaseChange(t *testing.T) {
	var executions atomic.Int32
	remote := sdk.NewServer(&sdk.Implementation{Name: "review", Version: "1"}, nil)
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "record", Description: "Record a review"}, func(_ context.Context, input struct {
		Value string `json:"value"`
	}) (string, error) {
		executions.Add(1)
		return input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scopemcp.Register(remote, executable); err != nil {
		t.Fatal(err)
	}
	var sessions atomic.Int32
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.Header.Get("Mcp-Session-Id") == "" {
			sessions.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	var modelName atomic.Value
	modelName.Store("")
	var steps atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		name := modelName.Load().(string)
		conversation := withoutDeferredCatalog(request)
		for _, part := range conversation[len(conversation)-1].Parts {
			if part.ToolResult != nil && part.ToolResult.Name == name {
				return completedTextResponse("recorded"), nil
			}
		}
		step := steps.Add(1)
		if step > 32 {
			return nil, errors.New("scripted model did not converge")
		}
		call := chat.ToolCall{ID: fmt.Sprintf("discover_%d", step), Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == name {
				call = chat.ToolCall{ID: fmt.Sprintf("record_%d", step), Name: name, Arguments: `{"value":"reviewed"}`}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := runtimeConfigWithRequiredDeps(t)
	cfg.ChatResolver = testChatResolver(client)
	t.Cleanup(func() {
		_ = filepath.WalkDir(filepath.Join(cfg.Stores.DataDirectory, "plugins"), func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	host, api := buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.Stores.Idempotency, IdempotencyNamespace: cfg.Stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		endpoint.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := endpoint.AwaitShutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	must := func(method delivery.Name, input any, key string) any {
		t.Helper()
		options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}, IdempotencyKey: key}
		result := endpoint.Invoke(t.Context(), method, input, options)
		if result.Failure != nil {
			t.Fatalf("%s: %v (%v)", method, result.Failure, errors.Unwrap(result.Failure))
		}
		return result.Value
	}

	source := t.TempDir()
	poApprovalWriteFiles(t, source, map[string]string{
		"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"review-package","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"requests":[{"capability":"tools.invoke","targets":["reviews/record"]}]}}}`,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"streamable-http","url":"` + server.URL + `"}}}`,
	})
	installed := must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	must(delivery.PluginsApprove, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, "approve")
	must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
	serverID := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	awaitTool := func(phase string) protocol.MCPTool {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			tools := must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &serverID}, "").(*protocol.Page[protocol.MCPTool])
			if len(tools.Data) == 1 {
				return tools.Data[0]
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: installation MCP tools = %+v", phase, tools.Data)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	modelName.Store(awaitTool("first release").ModelName)

	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion:    protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}},
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}, Title: "release approval"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(phase string) (string, []protocol.PendingInterruptSet) {
		t.Helper()
		started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Record the review."}}})
		if err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
		waitForRunEvents(t, collectRunEvents(events), phase)
		pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
		if err != nil {
			t.Fatalf("%s interrupts: %v", phase, err)
		}
		return started.RunID, pending.Data
	}
	finished := func(phase, runID string) {
		t.Helper()
		ref, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: runID})
		if err != nil || ref.Outcome == nil || ref.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("%s outcome = %+v, %v", phase, ref, err)
		}
	}

	runID, pending := run("first call")
	if len(pending) != 1 || len(pending[0].Interrupts) != 1 || pending[0].Interrupts[0].Type != protocol.InterruptApproval {
		t.Fatalf("first installation tool call did not prompt: %+v", pending)
	}
	if executions.Load() != 0 {
		t.Fatal("installation tool executed before approval")
	}
	_, events, err := api.ResumeRun(ctx, protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{
		ItemID: pending[0].Interrupts[0].ItemID,
		Response: protocol.InterruptResponseValue{
			Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			Remember: &protocol.RememberScope{Scope: protocol.RememberGlobal},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "approved call")
	finished("approved call", runID)
	if executions.Load() != 1 {
		t.Fatalf("approved call executions = %d", executions.Load())
	}
	rules := must(delivery.ApprovalListRules, protocol.ListApprovalRulesRequest{SessionID: session.ID}, "").(*protocol.ListApprovalRulesResult)
	if len(rules.Rules) != 1 || rules.Rules[0].Stale || rules.Rules[0].Tool.Type != protocol.ToolRefMCP ||
		rules.Rules[0].Tool.Server == nil || *rules.Rules[0].Tool.Server != serverID || rules.Rules[0].Tool.Name != "record" {
		t.Fatalf("standing rule = %+v", rules.Rules)
	}

	runID, pending = run("standing approval")
	if len(pending) != 0 {
		t.Fatalf("standing approval prompted again: %+v", pending)
	}
	finished("standing approval", runID)
	if executions.Load() != 2 {
		t.Fatalf("standing approval executions = %d", executions.Load())
	}

	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("next release"), 0600); err != nil {
		t.Fatal(err)
	}
	staged := must(delivery.PluginsStage, protocol.StagePluginRequest{InstallationID: installed.ID, Source: source}, "stage").(*protocol.PluginInstallation)
	if staged.Staged == nil || staged.Staged.Digest == installed.Selected.Digest {
		t.Fatalf("staged release = %+v", staged.Staged)
	}
	before := sessions.Load()
	selected := must(delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: staged.Staged.Digest}, "select").(*protocol.PluginInstallation)
	if selected.Selected.Digest != staged.Staged.Digest || selected.State != protocol.PluginInstallationUnapproved || selected.Presentation != protocol.PluginPresentationWithheld {
		t.Fatalf("release switch inherited approval of other code: %+v", selected)
	}
	rules = must(delivery.ApprovalListRules, protocol.ListApprovalRulesRequest{SessionID: session.ID}, "").(*protocol.ListApprovalRulesResult)
	if len(rules.Rules) != 1 || !rules.Rules[0].Stale {
		t.Fatalf("standing rule after release switch = %+v, want stale", rules.Rules)
	}
	if sessions.Load() != before {
		t.Fatal("release switch launched unreviewed code")
	}
	// The switch withdrew the old release's tools in the critical section that
	// committed it, so no Run admitted after it can freeze them.
	if tools := must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &serverID}, "").(*protocol.Page[protocol.MCPTool]); len(tools.Data) != 0 {
		t.Fatalf("release switch left the old release's tools live: %+v", tools.Data)
	}
	must(delivery.PluginsApprove, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: selected.Selected.Digest}, "approve-second")
	must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-second")
	// Enabling the reviewed release dials it in the background.
	if tool := awaitTool("second release"); tool.ModelName != modelName.Load().(string) {
		t.Fatalf("release switch changed the model-visible name: %q", tool.ModelName)
	}
	if sessions.Load() == before {
		t.Fatal("the reviewed release's tools were served without a new connection")
	}
	rules = must(delivery.ApprovalListRules, protocol.ListApprovalRulesRequest{SessionID: session.ID}, "").(*protocol.ListApprovalRulesResult)
	if len(rules.Rules) != 1 || !rules.Rules[0].Stale {
		t.Fatalf("standing rule after renewed approval = %+v, want stale", rules.Rules)
	}

	_, pending = run("after release switch")
	if len(pending) != 1 || len(pending[0].Interrupts) != 1 || pending[0].Interrupts[0].Type != protocol.InterruptApproval {
		t.Fatalf("stale approval was honored after the release switch: %+v executions=%d", pending, executions.Load())
	}
	if executions.Load() != 2 {
		t.Fatalf("installation tool executed under a stale approval: %d", executions.Load())
	}
}

func poApprovalWriteFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
