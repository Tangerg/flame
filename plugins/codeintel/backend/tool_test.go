package main

import (
	"context"
	"errors"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func attachedRun(t *testing.T) context.Context {
	t.Helper()
	ctx, err := pluginsdk.WithToolContext(t.Context(), pluginsdk.ToolContext{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func callTextTool(ctx context.Context, executable toolcontract.Tool, arguments string) (string, error) {
	binding, err := toolcontract.Bind(executable)
	if err != nil {
		return "", err
	}
	invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "test", Name: binding.Contract().Definition().Name, Arguments: arguments}, nil)
	if err != nil {
		return "", err
	}
	result, err := binding.Call(ctx, invocation)
	if err != nil {
		return "", err
	}
	text, found := result.Output.Text()
	if !found {
		return "", errors.New("test: expected text")
	}
	return text, nil
}

func requireDefiniteFailure(t *testing.T, err error, label string) {
	t.Helper()
	if _, known := toolcontract.ClassifyFailure(err); !known {
		t.Fatalf("%s: %v", label, err)
	}
}
