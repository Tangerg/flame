package client

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/samber/lo"
)

type PluginStreamer interface {
	StreamPlugin(context.Context, protocol.CallPluginRequest, protocol.PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error)
}

type PluginStreamingMethod[P, C, R any] struct {
	boundMethod
	caller PluginStreamer
}

func BindPluginStreamingMethod[P, C, R any](caller PluginStreamer, binding PluginBinding, method *pluginmcp.StreamingMethod[P, C, R]) (*PluginStreamingMethod[P, C, R], error) {
	if lo.IsNil(caller) || method == nil {
		return nil, errors.New("runtime: plugin streamer and typed method are required")
	}
	bound, err := bindMethod(binding, method.Descriptor())
	if err != nil {
		return nil, err
	}
	return &PluginStreamingMethod[P, C, R]{boundMethod: bound, caller: caller}, nil
}

func (m *PluginStreamingMethod[P, C, R]) Stream(ctx context.Context, input P, options protocol.PluginCallOptions) iter.Seq2[pluginmcp.StreamValue[C, R], error] {
	captured := options.Clone()
	request, err := m.request(input, captured)
	var consumed atomic.Bool
	return func(yield func(pluginmcp.StreamValue[C, R], error) bool) {
		if !consumed.CompareAndSwap(false, true) {
			yield(pluginmcp.StreamValue[C, R]{}, ErrClosed)
			return
		}
		if err != nil {
			yield(pluginmcp.StreamValue[C, R]{}, err)
			return
		}
		_, events, err := m.caller.StreamPlugin(ctx, request, captured)
		if err == nil {
			err = m.deliver(events, yield)
		}
		if err != nil {
			yield(pluginmcp.StreamValue[C, R]{}, err)
		}
	}
}

func (m *PluginStreamingMethod[P, C, R]) deliver(events iter.Seq2[protocol.PluginStreamEvent, error], yield func(pluginmcp.StreamValue[C, R], error) bool) error {
	if events == nil {
		return m.contractBreach(nil)
	}
	var final *R
	for event, err := range events {
		if err != nil {
			return err
		}
		if final != nil {
			return m.contractBreach(errors.New("runtime: plugin stream continued after its result"))
		}
		if err := protocol.ValidateWireTree(event); err != nil {
			return m.contractBreach(err)
		}
		switch event.Kind {
		case protocol.PluginStreamFailure:
			return pluginRefusal(event.Failure)
		case protocol.PluginStreamChunk:
			chunk, err := decodeOutput[C](m.method.ValidateChunk, event.Value)
			if err != nil {
				return m.contractBreach(err)
			}
			if !yield(pluginmcp.StreamChunk[R](chunk), nil) {
				return nil
			}
		case protocol.PluginStreamResult:
			result, err := decodeOutput[R](m.method.ValidateOutput, event.Value)
			if err != nil {
				return m.contractBreach(err)
			}
			final = &result
		}
	}
	if final == nil {
		return m.contractBreach(pluginmcp.ErrStreamUnconfirmed)
	}
	yield(pluginmcp.StreamResult[C](*final), nil)
	return nil
}
