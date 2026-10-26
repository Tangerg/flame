package executionctx

import (
	"context"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
)

// ModelCalls binds nested consumption to the admitted execution member. Finish
// projects original physical evidence through that member's existing commit pump.
type modelCalls interface {
	Start(context.Context, string, modelchoice.Selection) (func(context.Context, error) error, error)
}

type modelCallsKey struct{}

func WithModelCalls(ctx context.Context, calls modelCalls) context.Context {
	return context.WithValue(ctx, modelCallsKey{}, calls)
}
func ModelCallsFromContext(ctx context.Context) modelCalls {
	if ctx == nil {
		return nil
	}
	calls, _ := ctx.Value(modelCallsKey{}).(modelCalls)
	return calls
}
