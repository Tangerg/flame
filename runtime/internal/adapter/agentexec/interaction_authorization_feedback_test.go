package agentexec

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionPolicyRefusalPublishesItsExactReasonBeforeModelContinuation(t *testing.T) {
	const reason = "this operation is read-only; request permission before writing"
	for _, fromHook := range []bool{false, true} {
		name := "authorizer"
		if fromHook {
			name = "hook"
		}
		t.Run(name, func(t *testing.T) {
			toolCalls, modelCalls, publications := 0, 0, 0
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "write"}, func(context.Context, struct{}) (string, error) {
				toolCalls++
				return "unexpected", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var committed *chat.ToolResult
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				modelCalls++
				if modelCalls == 1 {
					return interactionToolResponse(chat.ToolCall{ID: "refused", Name: "write", Arguments: `{}`}, 1, 1), nil
				}
				matches := 0
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil && part.ToolResult.ID == "refused" {
							matches++
							if !reflect.DeepEqual(part.ToolResult, committed) {
								return nil, errors.New("refusal differs from the committed result")
							}
						}
					}
				}
				if matches != 1 || publications != 1 {
					return nil, errors.New("refusal was not committed exactly once before continuation")
				}
				return interactionUsageTextResponse("request permission", 1, 1), nil
			})
			policy := feedbackAuthorizer{decision: DenyTool(reason)}
			hooks := &feedbackHooks{}
			if fromHook {
				policy.decision = AllowTool()
				hooks.decision = DenyToolHook(reason)
			}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: policy, ToolHooks: hooks,
			})
			events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
				if batch, ok := fact.(runs.ToolResultsCommitted); ok {
					publications++
					if len(batch.Results) != 1 {
						return errors.New("unexpected result batch")
					}
					committed = batch.Results[0].ModelResult
				}
				return nil
			})
			if committed == nil || !committed.IsError || !reflect.DeepEqual(committed.Output, chat.NewTextToolOutput(reason)) {
				t.Fatalf("refusal lost its public reason: %#v", committed)
			}
			finished := payloadsOf[runs.ToolCallFinished](events)
			if len(finished) != 1 || finished[0].Failure == nil || *finished[0].Failure != (domaintool.Failure{Kind: domaintool.FailureDenied, Detail: reason}) {
				t.Fatalf("product refusal differs from public outcome: %#v", finished)
			}
			if toolCalls != 0 || hooks.after != 0 || modelCalls != 2 || len(payloadsOf[runs.AssistantMessageCompleted](events)) != 1 {
				t.Fatalf("refusal lifecycle: tool=%d after=%d model=%d", toolCalls, hooks.after, modelCalls)
			}
		})
	}
}

func TestInteractionAuthorizationErrorsDoNotPublishNestedOutcomes(t *testing.T) {
	inner, err := toolcontract.NewFailure(toolcontract.FailureConfig{
		Kind: toolcontract.FailureKindRejected, Output: chat.NewTextToolOutput("private nested refusal"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fromHook := range []bool{false, true} {
		name := "authorizer"
		if fromHook {
			name = "hook"
		}
		t.Run(name, func(t *testing.T) {
			toolCalls, modelCalls, publications := 0, 0, 0
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "write"}, func(context.Context, struct{}) (string, error) {
				toolCalls++
				return "unexpected", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			policy := feedbackAuthorizer{decision: AllowTool(), err: errors.Join(context.DeadlineExceeded, inner)}
			hooks := &feedbackHooks{}
			if fromHook {
				hooks.err, policy.err = policy.err, nil
			}
			model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				modelCalls++
				return interactionToolResponse(chat.ToolCall{ID: "unresolved", Name: "write", Arguments: `{}`}, 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: policy, ToolHooks: hooks,
			})
			events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
				if _, ok := fact.(runs.ToolResultsCommitted); ok {
					publications++
				}
				return nil
			})
			if toolCalls != 0 || modelCalls != 1 || publications != 0 || hooks.after != 0 || len(payloadsOf[runs.ToolCallFinished](events)) != 0 {
				t.Fatalf("authorization error became a result: tool=%d model=%d publications=%d", toolCalls, modelCalls, publications)
			}
			assertUnrecordedToolCallTerminal(t, events)
		})
	}
}

type feedbackAuthorizer struct {
	decision ToolAuthorizationDecision
	err      error
}

func (f feedbackAuthorizer) AuthorizeTool(context.Context, ToolAuthorizationRequest) (ToolAuthorizationDecision, error) {
	return f.decision, f.err
}

func (f feedbackAuthorizer) ResolveToolApproval(context.Context, ToolAuthorizationRequest, runs.ApprovalPrompt, interrupt.Resolution) (ToolAuthorizationDecision, error) {
	return f.decision, f.err
}

type feedbackHooks struct {
	decision InteractionToolHookDecision
	err      error
	after    int
}

func (f *feedbackHooks) BeforeToolUse(context.Context, InteractionToolHookInput) (InteractionToolHookDecision, error) {
	return f.decision, f.err
}

func (f *feedbackHooks) AfterToolUse(context.Context, InteractionToolHookInput) error {
	f.after++
	return nil
}
