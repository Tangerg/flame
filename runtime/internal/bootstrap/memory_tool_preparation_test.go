package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"iter"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestManagedMemoryToolUsesCapturedWorkspaceWithoutAnEmbeddingRole(t *testing.T) {
	for _, operation := range []string{memorycontract.SearchToolName, "add_memory"} {
		t.Run(operation, func(t *testing.T) {
			name := "memory_" + operation
			const fact = "prepared memory workspace fact"
			arguments := `{"query":"prepared memory"}`
			if operation == "add_memory" {
				arguments = `{"scope":"project","content":"` + fact + `"}`
			}
			var observed atomic.Bool
			model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				if len(request.Tools) == 0 {
					return completedTextResponse("memory"), nil
				}
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil && part.ToolResult.Name == name {
							encoded, err := json.Marshal(part.ToolResult.Output)
							if err != nil || part.ToolResult.IsError || !strings.Contains(string(encoded), fact) {
								return nil, fmt.Errorf("prepared Memory tool lost its captured target: %s: %v", encoded, err)
							}
							observed.Store(true)
							return completedTextResponse("recalled"), nil
						}
					}
				}
				call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
				for _, definition := range request.Tools {
					if definition.Name == name {
						call = chat.ToolCall{ID: "search", Name: name, Arguments: arguments}
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
			cfg.modelProviders = testChatResolver(client)
			cfg.approvalMode = approval.ModeYolo
			r := poMCPOpen(t, cfg)
			defer r.close()
			installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "memory"))
			server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
			r.poMCPTools(server, memoryToolsReady)
			if operation == memorycontract.SearchToolName {
				item := mustMemoryAdd(r, memoryAddRequest(r, memorycontract.AddV2Input{Scope: memorycontract.AddV2InputScopeProject, CWD: cfg.defaultWorkspacePath, Content: fact}), "prepare-fact")
				if item.Status != memorycontract.UpdateV2OutputStatusActive {
					t.Fatal("human-created fact was not active")
				}
			}
			session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Recall prepared Memory", Workspace: &protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}}, "memory-session").(*protocol.Session)
			started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Use memory to " + operation + " the prepared workspace fact."}}}, delivery.Options{IdempotencyKey: "memory-recall-start", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
			if started.Failure != nil {
				t.Fatal(started.Failure)
			}
			events := iter.Seq2[protocol.RunEvent, error](func(yield func(protocol.RunEvent, error) bool) {
				for event, err := range started.Events {
					if !yield(event.(protocol.RunEvent), err) {
						return
					}
				}
			})
			waitForRunEvents(t, collectRunEvents(events), "prepared Memory tool")
			if !observed.Load() {
				t.Fatal("actual Memory backend tool did not return its original workspace fact")
			}
			page := readMemoryPage(r, memorycontract.ListV2Input{Scope: memorycontract.ListV2InputScopeProject, CWD: cfg.defaultWorkspacePath})
			if len(page.Data) != 1 || page.Data[0].Content != fact {
				t.Fatalf("human read did not observe the model's memory owner: %+v", page.Data)
			}
		})
	}
}
