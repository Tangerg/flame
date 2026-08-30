package builtin

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func callTextTool(ctx context.Context, executable toolcontract.Tool, arguments string) (string, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return "", err
	}
	invocation, err := binding.Prepare(chat.ToolCall{
		ID: "test_call", Name: binding.Definition().Name, Arguments: arguments,
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
