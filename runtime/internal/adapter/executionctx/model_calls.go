package executionctx

import (
	"context"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

type modelCalls interface {
	Start(context.Context, string, flamemodel.Selection) (func(context.Context, error) error, error)
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
