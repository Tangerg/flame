package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	chathistory "github.com/Tangerg/scope/core/history"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestProtocolCompletesDirectToolResults(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		t.Run(fmt.Sprintf("delegated=%t", delegated), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("FLAME_HOME", home)
			var calls, executions atomic.Int32
			want := []chat.ToolResult{{ID: "answer_once", Name: "direct_answer", Output: chat.NewTextToolOutput("exact answer")}}
			model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				if len(request.Tools) == 0 {
					return completedTextResponse("title"), nil
				}
				sequence := calls.Add(1)
				if delegated && sequence == 4 {
					var results []chat.ToolResult
					for _, message := range request.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && part.ToolResult.Name == "delegate_task" {
								var output struct {
									Source            string            `json:"source"`
									DirectToolResults []chat.ToolResult `json:"direct_tool_results"`
									ModelCalls        uint64            `json:"model_calls"`
								}
								if err := json.Unmarshal(part.ToolResult.Output.Details, &output); err != nil {
									return nil, err
								}
								if output.Source != "direct_tool_results" || output.ModelCalls != 2 {
									return nil, fmt.Errorf("delegated completion changed: %+v", output)
								}
								results = output.DirectToolResults
							}
						}
					}
					if !reflect.DeepEqual(results, want) {
						return nil, fmt.Errorf("delegated direct results changed: %+v", results)
					}
					return completedTextResponse("root completed"), nil
				}
				call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:direct_answer"}`}
				for _, definition := range request.Tools {
					if definition.Name == "direct_answer" {
						call = chat.ToolCall{ID: "answer_once", Name: "direct_answer", Arguments: `{}`}
					}
				}
				if delegated && sequence == 1 {
					call = chat.ToolCall{ID: "delegate_once", Name: "delegate_task", Arguments: `{"summary":"answer","instructions":"return the direct answer"}`}
				}
				message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
				return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}}, nil
			})}
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, DefaultWorkspacePath: home})
			if err != nil {
				t.Fatal(err)
			}
			cfg := protocolRuntimeConfig(t, stores, model)
			cfg.ApprovalMode = approval.ModeYolo
			answer, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "direct_answer", Description: "Return the answer."}, func(context.Context, struct{}) (string, error) {
				executions.Add(1)
				return "exact answer", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.Resources), func(ctx context.Context, deps toolEnvironmentDependencies) (toolEnvironment, error) {
				source, err := mcpserver.ParseServerName("fixture")
				if err != nil {
					return toolEnvironment{}, err
				}
				deps.mcp.policy.Replace(mcpserver.NewToolPolicy([]mcpserver.Server{{Name: source, Enabled: true}}))
				environment, err := buildToolEnvironment(ctx, deps)
				if err == nil {
					environment.tools.Resolver.SetMCPTools([]toolcontract.Tool{directAnswerTool{answer}})
				}
				return environment, err
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := host.Close(); err != nil {
					t.Error(err)
				}
			})
			api, err := protocolHandler(host, home)
			if err != nil {
				t.Fatal(err)
			}
			ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
				ProtocolVersion:    protocol.ProtocolVersion,
				ClientCapabilities: &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}}},
			})
			session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "direct completion", Workspace: &protocol.WorkspaceRef{Path: home}})
			if err != nil {
				t.Fatal(err)
			}
			started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "answer"}}})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "direct completion")
			finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
			if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
				if err != nil {
					t.Fatal(err)
				}
				t.Fatalf("direct Run outcome = %+v", finished.Outcome)
			}
			wantCalls := int32(2)
			if delegated {
				wantCalls = 4
			}
			if calls.Load() != wantCalls || executions.Load() != 1 {
				t.Fatalf("model calls=%d, Tool executions=%d", calls.Load(), executions.Load())
			}
			if !delegated {
				history, err := stores.ChatHistory.Read(ctx, chathistory.ConversationID(session.ID))
				if err != nil {
					t.Fatal(err)
				}
				var results []chat.ToolResult
				for _, message := range history {
					if message.Role == chat.RoleAssistant && message.Text() != "" {
						t.Fatal("direct completion synthesized an assistant answer")
					}
					for _, part := range message.Parts {
						if part.ToolResult != nil && part.ToolResult.Name == "direct_answer" {
							results = append(results, part.ToolResult.Clone())
						}
					}
				}
				if !reflect.DeepEqual(results, want) {
					t.Fatalf("durable direct results = %+v", results)
				}
			}
		})
	}
}

type directAnswerTool struct{ toolcontract.Tool }

func (directAnswerTool) ReturnsDirectResult() bool         { return true }
func (directAnswerTool) MCPToolIdentity() (string, string) { return "fixture", "answer" }
