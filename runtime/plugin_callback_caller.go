package runtime

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/jsoninput"
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
	ref, err := c.reference(ctx)
	if err != nil {
		return nil, err
	}
	client, err := ConnectCallback(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, client.Close()) }()
	return client.CallPlugin(ctx, request, options)
}

func (c *PluginCallbackCaller) reference(ctx context.Context) (pluginsdk.CallbackRef, error) {
	if c == nil {
		return pluginsdk.CallbackRef{}, errors.New("runtime: plugin callback caller is required")
	}
	ref, refErr := pluginsdk.CallbackFromContext(ctx)
	if refErr != nil {
		if _, invocationErr := pluginsdk.InvocationFromContext(ctx); invocationErr == nil {
			return pluginsdk.CallbackRef{}, errors.New("runtime: an invoked plugin dependency requires its calling-owner callback")
		}
		ref = c.activation
	}
	return ref, nil
}

// StreamPlugin captures the original callback and envelope before returning a
// lazy sequence. Its empty opening conveys no remote business acknowledgement.
func (c *PluginCallbackCaller) StreamPlugin(ctx context.Context, request protocol.CallPluginRequest, options PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	ref, err := c.reference(ctx)
	if err != nil {
		return struct{}{}, nil, err
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return struct{}{}, nil, err
	}
	input, err := jsoninput.Parse([]byte(request.Input))
	if err != nil {
		return struct{}{}, nil, err
	}
	request.Input = string(input.JSON())
	options, err = captureStreamOptions(options)
	if err != nil {
		return struct{}{}, nil, err
	}
	var consumed atomic.Bool
	return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
		if !consumed.CompareAndSwap(false, true) {
			yield(protocol.PluginStreamEvent{}, ErrClosed)
			return
		}
		client, err := ConnectCallback(ctx, ref)
		if err != nil {
			yield(protocol.PluginStreamEvent{}, err)
			return
		}
		defer client.Close()
		_, events, err := client.StreamPlugin(ctx, request, options)
		if err != nil {
			yield(protocol.PluginStreamEvent{}, err)
			return
		}
		for event, err := range events {
			if !yield(event, err) || err != nil {
				return
			}
		}
	}, nil
}
