package client

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"sync/atomic"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/samber/lo"
)

type PluginStreamer interface {
	StreamPlugin(context.Context, protocol.CallPluginRequest, protocol.PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error)
}

type PluginStreamingMethod[P, C, R any] struct {
	caller  PluginStreamer
	binding PluginBinding
	method  *portable.Method
}

func BindPluginStreamingMethod[P, C, R any](caller PluginStreamer, binding PluginBinding, method *pluginmcp.StreamingMethod[P, C, R]) (*PluginStreamingMethod[P, C, R], error) {
	if lo.IsNil(caller) || method == nil {
		return nil, errors.New("runtime: plugin streamer and typed method are required")
	}
	required, err := admitPluginBinding(binding, method.Descriptor())
	if err != nil {
		return nil, err
	}
	return &PluginStreamingMethod[P, C, R]{caller: caller, binding: binding, method: required}, nil
}

// Stream freezes input and options before lazy delivery. The returned sequence
// has one consumer and never resubmits or changes its captured binding.
func (m *PluginStreamingMethod[P, C, R]) Stream(ctx context.Context, input P, options protocol.PluginCallOptions) iter.Seq2[pluginmcp.StreamValue[C, R], error] {
	admitted, inputErr := framevalue.EncodeJSON(input)
	capturedOptions, optionsErr := captureStreamOptions(options)
	var consumed atomic.Bool
	return func(yield func(pluginmcp.StreamValue[C, R], error) bool) {
		fail := func(err error) {
			if m != nil && m.method != nil && m.method.Kind() == portable.Command {
				err = errors.Join(ErrAcknowledgementUnknown, err)
			}
			yield(pluginmcp.StreamValue[C, R]{}, err)
		}
		if !consumed.CompareAndSwap(false, true) {
			yield(pluginmcp.StreamValue[C, R]{}, ErrClosed)
			return
		}
		if m == nil || m.method == nil || inputErr != nil || optionsErr != nil {
			yield(pluginmcp.StreamValue[C, R]{}, errors.Join(inputErr, optionsErr, errors.New("runtime: valid bound stream input and options are required")))
			return
		}
		if m.method.Kind() == portable.Query && (capturedOptions.IdempotencyKey != "" || capturedOptions.IdempotencyNamespace != "") {
			yield(pluginmcp.StreamValue[C, R]{}, errors.New("runtime: plugin query cannot carry a command identity"))
			return
		}
		if err := m.method.ValidateInput(admitted); err != nil {
			yield(pluginmcp.StreamValue[C, R]{}, err)
			return
		}
		request := protocol.CallPluginRequest{InstallationID: m.binding.InstallationID, Digest: m.binding.Digest, Contribution: m.binding.Contribution, Contract: string(m.binding.Contract.Identity().Bytes()), Method: m.method.Name(), Input: string(admitted.Bytes()), Target: m.binding.Target}
		_, events, err := m.caller.StreamPlugin(ctx, request, capturedOptions)
		if err != nil {
			yield(pluginmcp.StreamValue[C, R]{}, err)
			return
		}
		if events == nil {
			fail(protocol.ErrInvalidResponse)
			return
		}
		var final *R
		for event, err := range events {
			if err != nil {
				yield(pluginmcp.StreamValue[C, R]{}, err)
				return
			}
			if final != nil {
				fail(protocol.ErrInvalidResponse)
				return
			}
			if err := protocol.ValidateWireTree(event); err != nil {
				fail(errors.Join(protocol.ErrInvalidResponse, err))
				return
			}
			switch event.Kind {
			case protocol.PluginStreamFailure:
				yield(pluginmcp.StreamValue[C, R]{}, &framemcp.Refusal{Code: event.Failure.Code, Message: event.Failure.Message})
				return
			case protocol.PluginStreamChunk:
				if err := m.method.ValidateChunk([]byte(event.Value)); err != nil {
					fail(errors.Join(protocol.ErrInvalidResponse, err))
					return
				}
				var chunk C
				if err := json.Unmarshal([]byte(event.Value), &chunk, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
					fail(errors.Join(protocol.ErrInvalidResponse, err))
					return
				}
				if !yield(pluginmcp.StreamValue[C, R]{Chunk: &chunk}, nil) {
					return
				}
			case protocol.PluginStreamResult:
				if err := m.method.ValidateOutput([]byte(event.Value)); err != nil {
					fail(errors.Join(protocol.ErrInvalidResponse, err))
					return
				}
				final = new(R)
				if err := json.Unmarshal([]byte(event.Value), final, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
					fail(errors.Join(protocol.ErrInvalidResponse, err))
					return
				}
			}
		}
		if final == nil {
			fail(pluginmcp.ErrStreamUnconfirmed)
			return
		}
		yield(pluginmcp.StreamValue[C, R]{Result: final}, nil)
	}
}

func captureStreamOptions(options protocol.PluginCallOptions) (protocol.PluginCallOptions, error) {
	encoded, err := json.Marshal(options)
	if err != nil {
		return protocol.PluginCallOptions{}, err
	}
	var captured protocol.PluginCallOptions
	err = json.Unmarshal(encoded, &captured, json.RejectUnknownMembers(true), framevalue.Numbers())
	return captured, err
}
