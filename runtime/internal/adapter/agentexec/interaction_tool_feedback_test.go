package agentexec

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionToolFailureFeedbackMatchesDurableResult(t *testing.T) {
	partial, err := toolcontract.NewFailure(errors.New("partial write"), chat.NewTextToolOutput("one file written"))
	if err != nil {
		t.Fatal(err)
	}
	for name, cause := range map[string]error{
		"ordinary": errors.New("disk unavailable"),
		"partial":  partial,
		"denied":   toolcontract.ErrAuthorizationDenied,
	} {
		t.Run(name, func(t *testing.T) {
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
				Name: "write", Description: "Write the requested output.",
			}, func(context.Context, struct{}) (string, error) { return "", cause })
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
				if finished, ok := fact.(runs.ToolCallFinished); ok {
					committed = finished.ModelResult
				}
				return nil
			})
			if committed == nil || !committed.IsError || len(payloadsOf[runs.AssistantMessageCompleted](events)) != 1 {
				t.Fatalf("failed Tool feedback did not continue after commit: result=%+v events=%#v", committed, events)
			}
		})
	}
}
