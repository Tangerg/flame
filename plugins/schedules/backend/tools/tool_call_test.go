package tools

import (
	"context"
	"errors"
	"testing"

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

func requireDefiniteFailure(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was accepted", what)
	}
	var failure *toolcontract.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("%s returned an unclassified error (%v); the Host loses the Run tree on those", what, err)
	}
	if failure.Kind() != toolcontract.FailureKindFailed {
		t.Fatalf("%s failure kind = %q, want %q: nothing was refused permission, the call did not happen",
			what, failure.Kind(), toolcontract.FailureKindFailed)
	}
}
