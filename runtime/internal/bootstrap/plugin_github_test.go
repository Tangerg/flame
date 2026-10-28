package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestGitHubInstalledSecretLifecycleAndModelWritesShareThePublishedOwner(t *testing.T) {
	var name, arguments, refusalText atomic.Value
	name.Store("")
	arguments.Store(`{"repository":"../private","number":42}`)
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
				call = chat.ToolCall{ID: "github-" + target, Name: target, Arguments: arguments.Load().(string)}
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
	cfg.approvalMode = approval.ModeYolo
	cfg.chatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "github"))
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Inputs) != 1 || !installed.Selected.Inputs[0].Secret || installed.Selected.Inputs[0].Required {
		t.Fatalf("GitHub secret declaration is invalid: %+v", installed.Selected)
	}
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "github"}
	ready := func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "get_issue" }) && slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "create_issue" })
	}
	r.poMCPTools(server, ready)
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
	r.poMCPTools(server, ready)
	empty := ""
	configure(&empty, "configure-empty-github-token")
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerFailed)
	cleared := configure(nil, "clear-github-token")
	if cleared.InputStates["token"].Type != protocol.PluginInputUnset {
		t.Fatal("clearing the credential did not restore explicit public access")
	}
	tools := r.poMCPTools(server, ready)
	for _, tool := range tools {
		if tool.Name == "get_issue" {
			name.Store(tool.ModelName)
		}
	}
	if name.Load().(string) == "" {
		t.Fatal("installed GitHub Tool is missing")
	}
	contracts, err := portable.ParseCapability([]byte(testsupport.GitHubCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, method, key, code, prompt string
		input                           any
	}{
		{"get_issue", testsupport.GitHubGetIssueV1Method, "", "invalid_resource", "Inspect the requested issue.", testsupport.GitHubGetIssueV1Input{Repository: "../private", Number: 42}},
		{"create_issue", testsupport.GitHubCreateIssueV1Method, "github-create-without-token", "credential_required", "Create the requested issue.", testsupport.GitHubCreateIssueV1Input{Repository: "owner/repo", Title: "Requested issue"}},
	} {
		for _, tool := range tools {
			if tool.Name == test.name {
				name.Store(tool.ModelName)
			}
		}
		input, err := jsonvalue.Encode(test.input)
		if err != nil {
			t.Fatal(err)
		}
		arguments.Store(input.String())
		refusalText.Store("")
		request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "github-operations", Contract: contracts.Identity().String(), Method: test.method, Input: input.String(), Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}
		typed := r.must(delivery.PluginsCall, request, test.key).(*protocol.CallPluginResponse)
		if typed.Failure == nil || typed.Failure.Code != test.code {
			t.Fatalf("typed %s bypassed admission: %+v", test.name, typed)
		}
		if test.key != "" {
			replay := r.must(delivery.PluginsCall, request, test.key).(*protocol.CallPluginResponse)
			if replay.Failure == nil || replay.Failure.Code != typed.Failure.Code || replay.Failure.Message != typed.Failure.Message {
				t.Fatalf("installed original refusal was not recoverable: %+v", replay)
			}
		}
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "GitHub " + test.name}, "github-session-"+test.name).(*protocol.Session)
		run := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: test.prompt}}}, delivery.Options{IdempotencyKey: "github-run-" + test.name, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
			t.Fatalf("installed typed/Tool %s refusal diverged: outcome=%+v typed=%+v model=%s", test.name, finished.Outcome, typed.Failure, refusalText.Load())
		}
	}
}
