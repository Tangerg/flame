package mcp

import (
	"context"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// AdmitRequest is the boundary for additional MCP entrances, such as a
// consumer's Tool adapter. It retains original input before typed translation
// and admits the same callback and invocation evidence as typed methods.
func AdmitRequest(ctx context.Context, input jsonvalue.Value, metadata sdk.Meta) (context.Context, error) {
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

// StreamCapacity projects the original caller's transport budget. A consumer
// may apply it to its own aggregate; this does not allocate another budget.
func StreamCapacity(ctx context.Context) (int64, bool) {
	capacity, present := ctx.Value(streamCapacityKey{}).(int64)
	return capacity, present
}
