package toolset

import (
	"context"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// decorateCall replaces Call while preserving the inner capability declarations
// — the shared spine of read/mutation guards and post-mutation diagnostics.
func decorateCall(inner toolcontract.Tool, call func(context.Context, toolcontract.Invocation) (toolcontract.Result, error)) toolcontract.Tool {
	return preserveDeclarations(&callDecorator{inner: inner, call: call}, inner)
}

// callDecorator is the backing type for [decorateCall]: it overrides Call while
// delegating Definition plus optional tool-loop declarations to the wrapped
// tool, so a decorator stack preserves the inner tool's full contract.
type callDecorator struct {
	inner toolcontract.Tool
	call  func(context.Context, toolcontract.Invocation) (toolcontract.Result, error)
}

func (c *callDecorator) Definition() toolcontract.Definition { return c.inner.Definition() }

func (c *callDecorator) Call(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	return c.call(ctx, invocation)
}
