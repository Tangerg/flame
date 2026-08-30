package toolset

import (
	"context"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// decorateCall replaces Call while preserving the inner capability declarations
// — the shared spine of read/mutation guards and post-mutation diagnostics.
func decorateCall(inner toolcontract.Tool, call func(context.Context, toolcontract.Invocation) (chat.ToolOutput, error)) toolcontract.Tool {
	return &callDecorator{inner: inner, call: call}
}

// callDecorator is the backing type for [decorateCall]: it overrides Call while
// delegating Definition plus optional tool-loop declarations to the wrapped
// tool, so a decorator stack preserves the inner tool's full contract.
type callDecorator struct {
	inner toolcontract.Tool
	call  func(context.Context, toolcontract.Invocation) (chat.ToolOutput, error)
}

func (c *callDecorator) Definition() chat.ToolDefinition { return c.inner.Definition() }

func (c *callDecorator) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	return c.call(ctx, invocation)
}

// Unwrap exposes the wrapped tool so its optional tool-loop declarations — a
// keyed file tool's per-path conflict class, where its mutations land, a
// return-direct policy — survive the whole decorator stack. Only Call is
// overridden here.
func (c *callDecorator) Unwrap() toolcontract.Tool { return c.inner }
