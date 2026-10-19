package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/plugins/github/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestGitHubInstalledSecretLifecycleAndModelToolUseThePublishedReadOwner(t *testing.T) {
	var name, refusalText atomic.Value
	name.Store("")
	refusalText.Store("")
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("github fixture"), nil
		}
		target := name.Load().(string)
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == target {
					if !part.ToolResult.IsError || len(part.ToolResult.Output.Content) != 1 {
						t.Error("installed read lost its known refusal")
					} else {
						refusalText.Store(part.ToolResult.Output.Content[0].Text)
					}
					return completedTextResponse("resource refused"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "discover-github", Name: "search_tools", Arguments: `{"query":"select:` + target + `"}`}
		for _, definition := range request.Tools {
			if definition.Name == target {
				call = chat.ToolCall{ID: "github-read", Name: target, Arguments: `{"repository":"../private","number":42}`}
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
	cfg.ApprovalMode = approval.ModeYolo
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "github"))
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Inputs) != 1 || !installed.Selected.Inputs[0].Secret || installed.Selected.Inputs[0].Required {
		t.Fatalf("GitHub secret declaration is invalid: %+v", installed.Selected)
	}
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "github"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	configure := func(value *string, key string) *protocol.PluginInstallation {
		t.Helper()
		change := protocol.PluginValueChange{Type: protocol.PluginValueClear}
		if value != nil {
			change = protocol.PluginValueChange{Type: protocol.PluginValueSet, Value: value}
		}
		return r.must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, ValueChanges: map[string]protocol.PluginValueChange{"token": change}}, key).(*protocol.PluginInstallation)
	}
	token := "private-github-installation-fixture-token"
	configured := configure(&token, "configure-github-token")
	encoded, err := json.Marshal(configured)
	if err != nil || strings.Contains(string(encoded), token) || configured.InputStates["token"].Type != protocol.PluginInputConfigured {
		t.Fatalf("secret inspection leaked or lost presence: %s, %v", encoded, err)
	}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	empty := ""
	configure(&empty, "configure-empty-github-token")
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerFailed)
	cleared := configure(nil, "clear-github-token")
	if cleared.InputStates["token"].Type != protocol.PluginInputUnset {
		t.Fatal("clearing the credential did not restore explicit public access")
	}
	tools := r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	for _, tool := range tools {
		if tool.Name == "get_issue" {
			name.Store(tool.ModelName)
		}
	}
	if name.Load().(string) == "" {
		t.Fatal("installed GitHub Tool is missing")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode(rpc.IssueInput{Repository: "../private", Number: 42})
	if err != nil {
		t.Fatal(err)
	}
	typed := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "github-operations", Contract: contracts.Capability.Identity().String(), Method: contracts.Issue.Name(), Input: input.String(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}, "").(*protocol.CallPluginResponse)
	if typed.Failure == nil || typed.Failure.Code != "invalid_resource" {
		t.Fatalf("typed read bypassed resource admission: %+v", typed)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "GitHub read"}, "github-session").(*protocol.Session)
	run := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Inspect the requested issue."}}}, delivery.Options{IdempotencyKey: "github-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
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
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted || refusalText.Load().(string) != typed.Failure.Message {
		t.Fatalf("installed typed/Tool refusal diverged: outcome=%+v typed=%+v model=%s", finished.Outcome, typed.Failure, refusalText.Load())
	}
}
