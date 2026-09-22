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

func callRejectedTool(t *testing.T, ctx context.Context, executable toolcontract.Tool, arguments string) string {
	t.Helper()
	_, err := callTextTool(ctx, executable, arguments)
	failure, ok := errors.AsType[*toolcontract.Failure](err)
	if !ok || failure.Kind() != toolcontract.FailureKindRejected || failure.Validate() != nil {
		t.Fatalf("Call error = %v, want a valid Scope rejection", err)
	}
	text, textual := failure.Output().Text()
	if !textual {
		t.Fatal("refusal contains non-text output")
	}
	return text
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
	invocation, err := binding.Contract().Prepare(chat.ToolCall{
		ID: "test_call", Name: binding.Contract().Definition().Name, Arguments: arguments,
	})
	return binding, invocation, err
}
