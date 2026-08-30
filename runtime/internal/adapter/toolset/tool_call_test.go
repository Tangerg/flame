package toolset

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func callTextTool(ctx context.Context, executable toolcontract.Tool, arguments string) (string, error) {
	binding, invocation, err := prepareTestInvocation(executable, arguments)
	if err != nil {
		return "", err
	}
	output, err := binding.Call(ctx, invocation)
	if err != nil {
		return "", err
	}
	text, ok := output.Text()
	if !ok {
		return "", errors.New("toolset test: Tool output contains non-text content")
	}
	return text, nil
}

func mustTestInvocation(t *testing.T, executable toolcontract.Tool, arguments string) toolcontract.Invocation {
	t.Helper()
	_, invocation, err := prepareTestInvocation(executable, arguments)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func prepareTestInvocation(
	executable toolcontract.Tool,
	arguments string,
) (toolcontract.Binding, toolcontract.Invocation, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return toolcontract.Binding{}, toolcontract.Invocation{}, err
	}
	invocation, err := binding.Prepare(chat.ToolCall{
		ID: "test_call", Name: binding.Definition().Name, Arguments: arguments,
	})
	return binding, invocation, err
}
