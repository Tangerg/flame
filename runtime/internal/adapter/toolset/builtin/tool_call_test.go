package builtin

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func callTextTool(ctx context.Context, executable toolcontract.Tool, arguments string) (string, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return "", err
	}
	invocation, err := binding.Contract().Prepare(chat.ToolCall{
		ID: "test_call", Name: binding.Contract().Definition().Name, Arguments: arguments,
	})
	if err != nil {
		return "", err
	}
	output, err := binding.Call(ctx, invocation)
	if err != nil {
		return "", err
	}
	text, ok := output.Text()
	if !ok {
		return "", errors.New("builtin test: Tool output contains non-text content")
	}
	return text, nil
}

// attachedRun is the Run scope a Tool executes under: one session sharing a
// workspace for every call in a test.
func attachedRun(t *testing.T) context.Context {
	t.Helper()
	return attachedRunIn(t, t.TempDir())
}

func attachedRunIn(t *testing.T, workspace string) context.Context {
	t.Helper()
	return executionctx.WithScope(t.Context(), runs.ExecutionScope{CWD: workspace, WorkspaceCWD: workspace})
}
