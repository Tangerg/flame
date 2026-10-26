//go:build darwin

package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/backend/domain"
	"github.com/Tangerg/flame/plugins/sandbox/backend/rpc"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestSandboxInstalledNativeToolAndTypedCommandsKeepCoreRunOwnership(t *testing.T) {
	var name atomic.Value
	name.Store("")
	var toolResult atomic.Value
	toolResult.Store("")
	var observed atomic.Bool
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("native sandbox fixture"), nil
		}
		target := name.Load().(string)
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == target {
					encoded, err := json.Marshal(part.ToolResult)
					if err != nil {
						t.Error(err)
					}
					toolResult.Store(string(encoded))
					var result sandbox.Result
					if err := json.Unmarshal(part.ToolResult.Output.Details, &result); err != nil {
						t.Error(err)
					}
					if !part.ToolResult.IsError && result.Status == sandbox.Completed && result.ExitCode == 0 && result.Stdout.Content == "native-model-result" {
						observed.Store(true)
					}
					return completedTextResponse("sandbox completed"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "discover-sandbox", Name: "search_tools", Arguments: `{"query":"select:` + target + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == target {
				call = chat.ToolCall{ID: "execute-native", Name: target, Arguments: `{"script":"printf native-model-result","timeoutMillis":3000}`}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.ChatResolver = testChatResolver(client)
	cfg.ApprovalMode = approval.ModeYolo
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "sandbox"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "sandbox"}
	tools := r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	name.Store(tools[0].ModelName)
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode(rpc.ExecuteInput{Script: "printf native-typed-result; exit 5", TimeoutMillis: 3000})
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "sandbox-operations", Contract: contracts.Capability.Identity().String(), Method: contracts.Execute.Name(), Input: input.String(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}
	typed := r.must(delivery.PluginsCall, request, "native-typed").(*protocol.CallPluginResponse)
	var result sandbox.Result
	if err := json.Unmarshal([]byte(typed.Output), &result); err != nil || typed.Failure != nil || result.ExitCode != 5 || result.Stdout.Content != "native-typed-result" {
		t.Fatalf("installed native callback failed: %+v %+v %v", typed, result, err)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Native scratch work"}, "sandbox-session").(*protocol.Session)
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Execute the native sandbox fixture."}}}, delivery.Options{IdempotencyKey: "sandbox-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.Value.(*protocol.StartRunResponse).RunID}, "").(*protocol.RunRef)
	if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted || !observed.Load() {
		t.Fatalf("core did not own the completed Tool Run: outcome=%+v observed=%v result=%s", run.Outcome, observed.Load(), toolResult.Load())
	}
	r.close()
	r = poMCPOpen(t, cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	replay := r.must(delivery.PluginsCall, request, "native-typed").(*protocol.CallPluginResponse)
	if replay.Output != typed.Output || replay.Failure != nil {
		t.Fatalf("restart changed original native result: %+v", replay)
	}
}
