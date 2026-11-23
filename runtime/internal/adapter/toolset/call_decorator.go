package toolset

import (
	"context"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

func decorateCall(inner toolcontract.Tool, call func(ctx context.Context, next toolcontract.Caller, invocation toolcontract.Invocation) (toolcontract.Result, error)) toolcontract.Tool {
	return toolcontract.Wrap(inner, func(next toolcontract.Caller) toolcontract.Caller {
		return toolcontract.CallerFunc(func(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
			return call(ctx, next, invocation)
		})
	})
}
