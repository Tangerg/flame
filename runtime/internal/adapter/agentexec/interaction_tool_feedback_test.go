package agentexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionToolFailureFeedbackMatchesDurableResult(t *testing.T) {
	partial, err := toolcontract.NewFailure(toolcontract.FailureConfig{
		Kind: toolcontract.FailureKindFailed, Cause: errors.New("partial write"),
		Output: chat.NewTextToolOutput("one file written"),
	})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := toolcontract.NewFailure(toolcontract.FailureConfig{
		Kind: toolcontract.FailureKindRejected, Cause: context.DeadlineExceeded,
		Output: chat.ToolOutput{Content: []chat.ToolContent{{Kind: chat.PartText, Text: "write is not permitted"}}, Details: []byte(`{"permitted":false}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, failure := range map[string]*toolcontract.Failure{
		"partial": partial,
		"denied":  denied,
	} {
		t.Run(name, func(t *testing.T) {
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
				Name: "write", Description: "Write the requested output.",
			}, func(context.Context, struct{}) (string, error) { return "", fmt.Errorf("tool boundary: %w", failure) })
			if err != nil {
				t.Fatal(err)
			}
			var committed *chat.ToolResult
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				if !hasToolMessage(request.Messages) {
					return interactionToolResponse(chat.ToolCall{ID: "write_once", Name: "write", Arguments: `{}`}, 1, 1), nil
				}
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil && !reflect.DeepEqual(part.ToolResult, committed) {
							return nil, errors.New("model feedback differs from the committed Tool result")
						}
					}
				}
				return interactionUsageTextResponse("recovered", 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
				if batch, ok := fact.(runs.ToolResultsCommitted); ok {
					committed = batch.Results[0].ModelResult
				}
				return nil
			})
			if committed == nil || !committed.IsError || len(payloadsOf[runs.AssistantMessageCompleted](events)) != 1 {
				t.Fatalf("failed Tool feedback did not continue after commit: result=%+v events=%#v", committed, events)
			}
			if !reflect.DeepEqual(committed.Output, failure.Output()) {
				t.Fatalf("public failure output changed: got %#v, want %#v", committed.Output, failure.Output())
			}
		})
	}
}

func TestInteractionResponseLostAfterSideEffectRemainsUnknown(t *testing.T) {
	var writes, modelCalls, publications atomic.Int32
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "write", Description: "Commit an external write.",
	}, func(context.Context, struct{}) (string, error) {
		writes.Add(1)
		return "", errors.New("connection closed before response")
	})
	if err != nil {
		t.Fatal(err)
	}
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		modelCalls.Add(1)
		return interactionToolResponse(chat.ToolCall{ID: "external_write", Name: "write", Arguments: `{}`}, 1, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if _, ok := fact.(runs.ToolResultsCommitted); ok {
			publications.Add(1)
		}
		return nil
	})
	if writes.Load() != 1 || modelCalls.Load() != 1 || publications.Load() != 0 ||
		len(payloadsOf[runs.UnknownEffectsDetected](events)) != 1 || len(payloadsOf[runs.ToolCallFinished](events)) != 0 {
		t.Fatalf("uncertain write became known or was repeated: writes=%d model=%d publications=%d events=%#v", writes.Load(), modelCalls.Load(), publications.Load(), events)
	}
}

func TestInteractionRejectedToolFeedbackMatchesDurableResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		call   chat.ToolCall
		finish chat.FinishReason
	}{
		{name: "invalid arguments", call: chat.ToolCall{ID: "rejected", Name: "read", Arguments: `{"shell":"bash"}`}},
		{name: "malformed arguments", call: chat.ToolCall{ID: "rejected", Name: "read", Arguments: `{"path":`}},
		{name: "non-object arguments", call: chat.ToolCall{ID: "rejected", Name: "read", Arguments: `[]`}},
		{name: "unknown tool", call: chat.ToolCall{ID: "rejected", Name: "unavailable", Arguments: `{}`}},
		{name: "truncated call", call: chat.ToolCall{ID: "rejected", Name: "read", Arguments: `{"path":"file"}`}, finish: chat.FinishReasonLength},
	} {
		t.Run(test.name, func(t *testing.T) {
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
				Name: "read", Description: "Read a file.",
			}, func(context.Context, struct {
				Path string `json:"path"`
			}) (string, error) {
				t.Error("rejected tool executed")
				return "", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var committed *chat.ToolResult
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				if !hasToolMessage(request.Messages) {
					response := interactionToolResponse(test.call, 1, 1)
					if test.finish != "" {
						response.Output.FinishReason = test.finish
					}
					return response, nil
				}
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil && !reflect.DeepEqual(part.ToolResult, committed) {
							return nil, errors.New("rejected Tool feedback reached the model before durable settlement")
						}
					}
				}
				return interactionUsageTextResponse("recovered", 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
				if batch, ok := fact.(runs.ToolResultsCommitted); ok {
					committed = batch.Results[0].ModelResult
				}
				return nil
			})
			if committed == nil || !committed.IsError || len(payloadsOf[runs.AssistantMessageCompleted](events)) != 1 {
				t.Fatalf("rejected Tool feedback did not continue after commit: result=%+v", committed)
			}
		})
	}
}
