package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
	scopemetadata "github.com/Tangerg/scope/core/metadata"
)

const InvocationMetadataKey = "io.github.tangerg.flame/invocation"

type invocationKey struct{}

func WithInvocation(ctx context.Context, invocation capability.Invocation) (context.Context, error) {
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, invocationKey{}, invocation), nil
}

func InvocationFromContext(ctx context.Context) (capability.Invocation, error) {
	invocation, found := ctx.Value(invocationKey{}).(capability.Invocation)
	if !found {
		return capability.Invocation{}, errors.New("plugin: calling-owner invocation is required")
	}
	return invocation, invocation.Validate()
}

func invocationMetadata(ctx context.Context, metadata sdk.ParamsBase) (sdk.ParamsBase, error) {
	invocation, found := ctx.Value(invocationKey{}).(capability.Invocation)
	if !found {
		return metadata, nil
	}
	if supplied, present := metadata.Meta[InvocationMetadataKey]; present {
		var value capability.Invocation
		if err := json.Unmarshal(supplied, &value, json.RejectUnknownMembers(true)); err != nil || value != invocation {
			return sdk.ParamsBase{}, errors.New("plugin: metadata cannot replace its calling-owner context")
		}
	}
	metadata.Meta = sdk.Meta(scopemetadata.Map(metadata.Meta).Clone())
	if err := metadata.Set(InvocationMetadataKey, invocation); err != nil {
		return sdk.ParamsBase{}, err
	}
	return metadata, nil
}

func bindInvocationMetadata(ctx context.Context, metadata sdk.Meta, method *capability.Method) (context.Context, error) {
	value, found := metadata[InvocationMetadataKey]
	if !found {
		return ctx, nil
	}
	var invocation capability.Invocation
	if err := json.Unmarshal(value, &invocation, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if owner, present := ctx.Value(invocationKey{}).(capability.Invocation); present {
		if invocation != owner {
			return nil, errors.New("plugin: callback cannot replace its calling-owner context")
		}
		return ctx, nil
	}
	if method != nil && method.Name() != ReceiptMethod && invocation.Target.Scope != method.Descriptor().Target {
		return nil, errors.New("plugin: invocation target differs from the method contract")
	}
	return WithInvocation(ctx, invocation)
}
