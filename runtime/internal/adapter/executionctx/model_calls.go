package executionctx

import (
	"context"

	modelchoice "github.com/Tangerg/flame/runtime/model"
)

// ModelCalls binds nested consumption to the admitted execution member. Finish
// projects original physical evidence through that member's existing commit pump.
type ModelCalls interface {
	Start(context.Context, string, modelchoice.Selection) (func(context.Context, error) error, error)
}

type modelCallsKey struct{}

func WithModelCalls(ctx context.Context, calls ModelCalls) context.Context {
	return context.WithValue(ctx, modelCallsKey{}, calls)
}
func ModelCallsFromContext(ctx context.Context) ModelCalls {
	if ctx == nil {
		return nil
	}
	calls, _ := ctx.Value(modelCallsKey{}).(ModelCalls)
	return calls
}
