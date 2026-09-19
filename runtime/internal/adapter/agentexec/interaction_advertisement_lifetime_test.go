package agentexec

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionExecutorClosesToolAdvertisementAfterReturn(t *testing.T) {
	var retained context.Context
	capture, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "capture", Description: "Capture this invocation's context.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		retained = ctx
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
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "capture_call", Name: "capture", Arguments: `{}`}, 1, 1),
		interactionTextResponse("done"),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{capture}, Deferred: []toolcontract.Tool{hidden},
		}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || retained == nil {
		t.Fatalf("Tool did not complete: %+v", ended)
	}
	if err := interaction.AdvertiseTools(retained, "hidden"); !errors.Is(err, interaction.ErrToolAdvertisementUnavailable) {
		t.Fatalf("advertisement after Tool return = %v, want unavailable", err)
	}
}
