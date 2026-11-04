package client

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

// PluginCallbackCaller binds background work to setup's activation and nested
// operations to their calling-owner callback, preserving original context.
type PluginCallbackCaller struct{ activation pluginmcp.CallbackRef }

func NewPluginCallbackCaller(activation pluginmcp.CallbackRef) (*PluginCallbackCaller, error) {
	if err := activation.Validate(); err != nil {
		return nil, err
	}
	return &PluginCallbackCaller{activation: activation}, nil
}

func (c *PluginCallbackCaller) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (result *protocol.CallPluginResponse, err error) {
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

func (c *PluginCallbackCaller) reference(ctx context.Context) (pluginmcp.CallbackRef, error) {
	if c == nil {
		return pluginmcp.CallbackRef{}, errors.New("runtime: plugin callback caller is required")
	}
	ref, refErr := pluginmcp.CallbackFromContext(ctx)
	if refErr != nil {
		if _, invocationErr := pluginmcp.InvocationFromContext(ctx); invocationErr == nil {
			return pluginmcp.CallbackRef{}, errors.New("runtime: an invoked plugin dependency requires its calling-owner callback")
		}
		ref = c.activation
	}
	return ref, nil
}

// StreamPlugin captures the original callback and envelope before returning a
// lazy sequence. Its empty opening conveys no remote business acknowledgement.
func (c *PluginCallbackCaller) StreamPlugin(ctx context.Context, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	ref, err := c.reference(ctx)
	if err != nil {
		return struct{}{}, nil, err
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return struct{}{}, nil, err
	}
	input, err := framevalue.ParseJSON([]byte(request.Input))
	if err != nil {
		return struct{}{}, nil, err
	}
	request.Input = string(input.Bytes())
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
