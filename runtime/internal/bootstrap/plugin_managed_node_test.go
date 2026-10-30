package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestManagedNodeInstalledOperationAndOrdinaryToolUseTheirCallingHost(t *testing.T) {
	source := t.TempDir()
	build := exec.CommandContext(t.Context(), "node", "../../../packages/backend-sdk/testing/build.mjs", source, "../../../runtime/internal/testsupport/testdata/managed-backend.ts")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build managed fixture: %s: %v", output, err)
	}
	method, err := qimumcp.NewMethod[protocol.UsageSummaryRequest, protocol.UsageSummary](qimumcp.MethodConfig{Name: "fixture.usage", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read core accounting through the calling host."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "flame.fixture.usage", Composition: portable.Collection, Scope: extension.ScopeInstallation}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := json.Marshal(contract.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"plugin.json":                              `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"managed-probe","version":"1.0.0","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
		"io.github.tangerg.flame/contract.json":    string(descriptor),
		"io.github.tangerg.flame/declaration.json": `{"protocol":"` + pluginsdk.DeclarationProtocol + `","managedBackends":[{"id":"managed","runtime":"node","entry":"start.mjs"}],"contributions":[{"id":"probe","server":"managed","export":"probe","contract":"io.github.tangerg.flame/contract.json"}]}`,
	} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	authorPluginPackage(t, source)
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	var toolName string
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("auxiliary"), nil
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == toolName {
					return completedTextResponse("observed"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "managed-probe-call", Name: "search_tools", Arguments: `{"query":"select:` + toolName + `"}`}
		if slices.ContainsFunc(request.Tools, func(tool chat.ToolDefinition) bool { return tool.Name == toolName }) {
			call.Name, call.Arguments = toolName, `{}`
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Servers) != 1 {
		t.Fatalf("managed declaration did not admit: %+v", installed)
	}
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "managed"}
	tools := r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	toolName = tools[0].ModelName
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "probe", Contract: contract.Identity().String(), Method: method.Name(), Input: `{"sinceDays":1}`, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}
	response := r.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
	var summary protocol.UsageSummary
	if response.Failure != nil || json.Unmarshal([]byte(response.Output), &summary) != nil || summary.ObservedAt.IsZero() || summary.Since == nil {
		t.Fatalf("managed operation did not reach Core: %+v", response)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Managed Tool"}, "session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Read accounting with the managed tool."}}}, delivery.Options{IdempotencyKey: "run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	run := started.Value.(*protocol.StartRunResponse)
	items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: run.RunID}}, "").(*protocol.ListItemsResponse)
	data, err := json.Marshal(items)
	if err != nil || !strings.Contains(string(data), "captured-tool-owner:") {
		t.Fatalf("ordinary managed Tool did not retain its call-scoped grant: %s: %v", data, err)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	r.close()
}
