package execution

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionToolUsesScopeAdmittedArguments(t *testing.T) {
	for _, arguments := range []string{"", " \n\t", "{}"} {
		t.Run(fmt.Sprintf("arguments=%q", arguments), func(t *testing.T) {
			var executions, modelCalls int
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
				Name: "fixture_inspect", Description: "Inspect without additional arguments.",
			}, func(context.Context, struct{}) (string, error) {
				executions++
				return "inspected", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				modelCalls++
				if hasToolMessage(request.Messages) {
					return interactionTextResponse("done"), nil
				}
				return interactionToolResponse(chat.ToolCall{ID: "inspect_call", Name: "fixture_inspect", Arguments: arguments}, 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "inspect")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted || executions != 1 || modelCalls != 2 {
				t.Fatalf("admitted arguments failed: ends=%+v executions=%d model=%d", ends, executions, modelCalls)
			}
			starts := payloadsOf[runs.ToolCallStarted](events)
			if len(starts) != 1 || starts[0].Arguments != "{}" {
				t.Fatalf("effective arguments = %+v", starts)
			}
			results := payloadsOf[runs.ToolCallFinished](events)
			if len(results) != 1 || results[0].ModelResult == nil || results[0].ModelResult.IsError {
				t.Fatalf("known tool result missing: %+v", results)
			}
		})
	}
}

func TestTransformedToolProposalIsAdmittedBeforeApprovalAndExecution(t *testing.T) {
	for _, final := range []string{`{"resource":"rewritten"}`, `{"resource":17}`} {
		t.Run(final, func(t *testing.T) {
			var authorized ToolAuthorizationRequest
			var executed string
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
				Name: "fixture_resource", Description: "Operate on a named resource.",
			}, func(_ context.Context, input resourceInput) (string, error) {
				executed = input.Resource
				return executed, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			provider := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				if hasToolMessage(request.Messages) {
					return interactionTextResponse("done"), nil
				}
				return interactionToolResponse(chat.ToolCall{ID: "proposal", Name: "fixture_resource", Arguments: `{"resource":"original"}`}, 1, 1), nil
			})
			decorated := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				response, err := provider.Call(ctx, request)
				if err == nil && !hasToolMessage(request.Messages) {
					response = response.Clone()
					response.Output.Message.Parts[0] = chat.NewToolCallPart(chat.ToolCall{ID: "proposal", Name: "fixture_resource", Arguments: final})
				}
				return response, err
			})
			executor := newObservedTestInteractionExecutor(t, decorated, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "resource")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: capturingToolAuthorizer{request: &authorized},
			})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			ends := payloadsOf[runs.SegmentEnded](events)
			results := payloadsOf[runs.ToolCallFinished](events)
			if len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted || len(results) != 1 || results[0].ModelResult == nil {
				t.Fatalf("final proposal did not settle through the Tool boundary: ends=%+v results=%+v", ends, results)
			}
			if final == `{"resource":17}` {
				if authorized.CallID != "" || executed != "" || !results[0].ModelResult.IsError {
					t.Fatalf("invalid final proposal reached approval or execution: %+v, %q", authorized, executed)
				}
				return
			}
			starts := payloadsOf[runs.ToolCallStarted](events)
			if authorized.Arguments.Canonical() != final || executed != "rewritten" || len(starts) != 1 || starts[0].Arguments != final || results[0].ModelResult.IsError {
				t.Fatalf("final proposal diverged across approval, execution or history: %+v, %q, %+v", authorized, executed, starts)
			}
		})
	}
}

func TestInteractionToolSchedulingUsesSafeArguments(t *testing.T) {
	for _, policy := range []string{"hook rewrite", "authorization rewrite", "immutable arguments"} {
		t.Run(policy, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var active, maximum, executions atomic.Int32
				release := make(chan struct{})
				executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
					Name: "fixture_resource", Description: "Operate on a named resource.",
				}, func(_ context.Context, input resourceInput) (string, error) {
					executions.Add(1)
					current := active.Add(1)
					for previous := maximum.Load(); current > previous; previous = maximum.Load() {
						if maximum.CompareAndSwap(previous, current) {
							break
						}
					}
					defer active.Add(-1)
					<-release
					return input.Resource, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
					if hasToolMessage(request.Messages) {
						return interactionTextResponse("done"), nil
					}
					return interactionToolBatchResponse([]chat.ToolCall{
						{ID: "a", Name: "fixture_resource", Arguments: `{"resource":"A"}`},
						{ID: "b", Name: "fixture_resource", Arguments: `{"resource":"B"}`},
					}, 1, 1), nil
				})
				config := InteractionExecutorConfig{
					ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "resource")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{resourceScheduledTool{executable}}}},
					ToolInterpreter: immutableToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
					MaxConcurrentToolCalls: intPointer(2),
				}
				switch policy {
				case "hook rewrite":
					config.ToolHooks = rewriteResourceHook{}
				case "authorization rewrite":
					config.ToolInterpreter = testInteractionToolInterpreter{}
					config.ToolAuthorizer = rewriteResourceAuthorizer{}
				}
				executor := newObservedTestInteractionExecutor(t, model, config)
				finished := make(chan []runs.ExecutorEvent, 1)
				go func() {
					finished <- runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(runs.ExecutionFact) error { return nil })
				}()
				synctest.Wait()
				wantConcurrency := int32(1)
				if policy == "immutable arguments" {
					wantConcurrency = 2
				}
				if got := active.Load(); got != wantConcurrency {
					t.Errorf("active resource operations = %d, want %d", got, wantConcurrency)
				}
				close(release)
				events := <-finished
				ends := payloadsOf[runs.SegmentEnded](events)
				if len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted || executions.Load() != 2 || maximum.Load() != wantConcurrency {
					t.Fatalf("end=%+v executions=%d concurrency=%d", ends, executions.Load(), maximum.Load())
				}
				for _, result := range payloadsOf[runs.ToolCallFinished](events) {
					text, _ := result.ModelResult.Output.Text()
					if policy != "immutable arguments" && text != "C" {
						t.Fatalf("executed original arguments: %q", text)
					}
				}
			})
		})
	}
}

type resourceInput struct {
	Resource string `json:"resource"`
}

type resourceScheduledTool struct{ toolcontract.Tool }

func (resourceScheduledTool) ConcurrencyPolicy() func(toolcontract.Invocation) (string, bool) {
	return func(invocation toolcontract.Invocation) (string, bool) {
		var input resourceInput
		if json.Unmarshal(invocation.Arguments(), &input) != nil {
			return "", false
		}
		return input.Resource, true
	}
}

type immutableToolInterpreter struct{ testInteractionToolInterpreter }

func (immutableToolInterpreter) UsesStandardPolicy(domaintool.Ref) bool { return false }

type rewriteResourceHook struct{}

func (rewriteResourceHook) BeforeToolUse(context.Context, InteractionToolHookInput) (InteractionToolHookDecision, error) {
	arguments, err := domaintool.ParseArguments(`{"resource":"C"}`)
	return allowToolHook(false, &arguments), err
}

func (rewriteResourceHook) AfterToolUse(context.Context, InteractionToolHookInput) error { return nil }

type rewriteResourceAuthorizer struct{ allowInteractionTools }

func (rewriteResourceAuthorizer) AuthorizeTool(context.Context, ToolAuthorizationRequest) (ToolAuthorizationDecision, error) {
	arguments, err := domaintool.ParseArguments(`{"resource":"C"}`)
	return allowToolWithArguments(arguments), err
}

func TestInteractionDirectToolCompletionUsesCommittedResults(t *testing.T) {
	var modelCalls, executions atomic.Int32
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "fixture_answer", Description: "Return the answer directly.",
	}, func(_ context.Context, input resourceInput) (string, error) {
		executions.Add(1)
		return input.Resource, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		modelCalls.Add(1)
		return interactionToolBatchResponse([]chat.ToolCall{
			{ID: "first", Name: "fixture_answer", Arguments: `{"resource":"first answer"}`},
			{ID: "second", Name: "fixture_answer", Arguments: `{"resource":"second answer"}`},
		}, 1, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "answer")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{directResultTool{executable}}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	var committed []chat.ToolResult
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if batch, ok := fact.(runs.ToolResultsCommitted); ok {
			for _, result := range batch.Results {
				committed = append(committed, result.ModelResult.Clone())
			}
		}
		return nil
	})
	ends := payloadsOf[runs.SegmentEnded](events)
	if len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted || modelCalls.Load() != 1 || executions.Load() != 2 {
		t.Fatalf("direct completion: ends=%+v model=%d tools=%d", ends, modelCalls.Load(), executions.Load())
	}
	completed := payloadsOf[runs.ModelCallCompleted](events)
	if len(completed) != 1 || len(completed[0].Message.Parts) != 2 {
		t.Fatalf("direct completion changed the model response: %+v", completed)
	}
	for _, part := range completed[0].Message.Parts {
		if part.Kind != chat.PartToolCall {
			t.Fatalf("direct completion synthesized assistant content: %+v", part)
		}
	}
	want := []chat.ToolResult{
		{ID: "first", Name: "fixture_answer", Output: chat.NewTextToolOutput("first answer")},
		{ID: "second", Name: "fixture_answer", Output: chat.NewTextToolOutput("second answer")},
	}
	if !reflect.DeepEqual(committed, want) {
		t.Fatalf("direct results = %+v, want %+v", committed, want)
	}
}

type directResultTool struct{ toolcontract.Tool }

func (directResultTool) ReturnsDirectResult() bool { return true }
