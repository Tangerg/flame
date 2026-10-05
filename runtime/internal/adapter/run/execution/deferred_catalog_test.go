package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestEmptyDeferredSetSendsNoCatalogMessage(t *testing.T) {
	if trailer := deferredCatalogMessage(toolset.Manifest{}); trailer != nil {
		t.Fatalf("empty deferred set projected %d catalog messages", len(trailer))
	}
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "write"}, func(context.Context, struct{}) (string, error) {
		return "written", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		calls++
		last := request.Messages[len(request.Messages)-1]
		for _, message := range request.Messages {
			if strings.Contains(message.Text(), "Deferred tools for this run") {
				return nil, errors.New("a Run without deferred Tools received a catalog message")
			}
		}
		if last.Role != chat.RoleUser {
			return nil, errors.New("the request does not end with the current user message")
		}
		return interactionUsageTextResponse("done", 1, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "write")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(runs.ExecutionFact) error { return nil })
	if calls != 1 {
		t.Fatalf("model calls = %d, want 1", calls)
	}
}
