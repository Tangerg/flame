package runtime

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
)

// PluginCallbackCaller binds background work to setup's activation and nested
// operations to their calling-owner callback, preserving original context.
type PluginCallbackCaller struct{ activation pluginsdk.CallbackRef }

func NewPluginCallbackCaller(activation pluginsdk.CallbackRef) (*PluginCallbackCaller, error) {
	if err := activation.Validate(); err != nil {
		return nil, err
	}
	return &PluginCallbackCaller{activation: activation}, nil
}

func (c *PluginCallbackCaller) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, options PluginCallOptions) (result *protocol.CallPluginResponse, err error) {
	if c == nil {
		return nil, errors.New("runtime: plugin callback caller is required")
	}
	ref, refErr := pluginsdk.CallbackFromContext(ctx)
	if refErr != nil {
		if _, invocationErr := pluginsdk.InvocationFromContext(ctx); invocationErr == nil {
			return nil, errors.New("runtime: an invoked plugin dependency requires its calling-owner callback")
		}
		ref = c.activation
	}
	client, err := ConnectCallback(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, client.Close()) }()
	return client.CallPlugin(ctx, request, options)
}
