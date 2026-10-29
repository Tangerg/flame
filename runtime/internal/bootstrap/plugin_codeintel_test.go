package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestInstalledCodeIntelligenceFormatsThroughTheRuntimeFileOwner(t *testing.T) {
	var target, observed atomic.Value
	target.Store("active.json")
	observed.Store("")
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		read := false
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult == nil {
					continue
				}
				switch part.ToolResult.Name {
				case "read":
					read = true
				case "edit":
					text, _ := part.ToolResult.Output.Text()
					observed.Store(text)
					return completedTextResponse("edit completed"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "read-source", Name: "read", Arguments: `{"path":"` + target.Load().(string) + `"}`}
		if read {
			call = chat.ToolCall{ID: "edit-source", Name: "edit", Arguments: `{"path":"` + target.Load().(string) + `","old_string":"1","new_string":"2"}`}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.chatResolver = testChatResolver(client)
	cfg.approvalMode = approval.ModeYolo
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "codeintel"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "codeintel"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "lsp" })
	runEdit := func(name string) string {
		t.Helper()
		target.Store(name)
		path := filepath.Join(cfg.defaultWorkspacePath, name)
		if err := os.WriteFile(path, []byte(`{"value":1}`), 0600); err != nil {
			t.Fatal(err)
		}
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: name}, "session-"+name).(*protocol.Session)
		started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Change the value."}}}, delivery.Options{IdempotencyKey: "run-" + name, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if started.Failure != nil {
			t.Fatal(started.Failure)
		}
		for _, err := range started.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.Value.(*protocol.StartRunResponse).RunID}, "").(*protocol.RunRef)
		if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("edit outcome: %+v", run)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	if content := runEdit("active.json"); content != "{\n  \"value\": 2\n}\n" {
		t.Fatalf("selected formatter: %q, result: %s", content, observed.Load())
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-codeintel")
	if content := runEdit("disabled.json"); content != `{"value":2}` {
		t.Fatalf("disabled plugin still formatted: %q", content)
	}
	if !strings.Contains(observed.Load().(string), "Unavailable:") {
		t.Fatalf("selected unavailable plugin was hidden: %s", observed.Load())
	}
}
