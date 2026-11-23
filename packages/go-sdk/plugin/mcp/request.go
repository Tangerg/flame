package mcp

import (
	"context"

	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func AdmitRequest(ctx context.Context, input framevalue.JSON, metadata sdk.Meta) (context.Context, error) {
	if err := input.Validate(); err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, requestInputKey{}, input)
	ctx, err := bindInvocationMetadata(ctx, metadata, nil)
	if err != nil {
		return ctx, err
	}
	return bindCallbackMetadata(ctx, metadata)
}

func StreamCapacity(ctx context.Context) (int64, bool) {
	capacity, present := ctx.Value(streamCapacityKey{}).(int64)
	return capacity, present
}
