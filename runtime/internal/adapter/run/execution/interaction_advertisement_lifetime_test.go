package execution

import (
	"context"
	"testing"

	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionExecutorDoesNotAdvertiseToolsWithoutTheCapability(t *testing.T) {
	capture, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "capture", Description: "Return a value without advertising Tools.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "hidden", Description: "Read a deferred value.",
	}, func(context.Context, struct{}) (string, error) { return "value", nil })
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		calls++
		for _, definition := range request.Tools {
			if definition.Name == "hidden" {
				t.Error("plain Tool advertised a deferred Tool")
			}
		}
		if calls == 1 {
			return interactionToolResponse(chat.ToolCall{ID: "capture_call", Name: "capture", Arguments: `{}`}, 1, 1), nil
		}
		return interactionTextResponse("done"), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "capture"), testsupport.A2ATool(t, "hidden")}, manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{capture}, Deferred: []toolcontract.Tool{hidden},
		}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || calls != 2 {
		t.Fatalf("Tool did not complete: %+v", ended)
	}
}
