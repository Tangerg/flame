package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"

	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/scope/core/chat"
)

func ModelStreamMethod() (*pluginmcp.StreamingMethod[chat.Request, chat.ResponseDelta, ModelResult], error) {
	return pluginmcp.NewStreamingMethod[chat.Request, chat.ResponseDelta, ModelResult](portable.MethodConfig{
		Name: "flame.model/stream.v1", Kind: portable.Command, Target: extension.ScopeCall,
		Description: "Consume the captured model's canonical stream and final outcome.",
	})
}

func StreamModel(ctx context.Context, ref pluginmcp.CallbackRef, id pluginmcp.CommandID, request *chat.Request) iter.Seq2[pluginmcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
	method, err := ModelStreamMethod()
	if err == nil {
		err = request.Validate()
	}
	if err != nil {
		return func(yield func(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
		}
	}
	return pluginmcp.StreamHost(ctx, ref, method, pluginmcp.CommandID(id), *request.Clone())
}

func NewModelStreamCallback(stream func(context.Context, pluginmcp.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error]) (pluginmcp.Callback, error) {
	if stream == nil {
		return pluginmcp.Callback{}, errors.New("plugin: model stream owner is required")
	}
	method, err := ModelStreamMethod()
	if err != nil {
		return pluginmcp.Callback{}, err
	}
	return pluginmcp.NewStreamingCallback(method, func(ctx context.Context, command pluginmcp.Command[chat.Request]) iter.Seq2[pluginmcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
		return func(yield func(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			if err := command.Input.Validate(); err != nil {
				yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			command.Input = *command.Input.Clone()
			ctx, err := modelCallContext(ctx, string(command.ID))
			if err != nil {
				yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			modelStreamResult(ctx, func() iter.Seq2[*chat.ResponseDelta, error] { return stream(ctx, command) })(yield)
		}
	})
}

func modelStreamResult(ctx context.Context, source func() iter.Seq2[*chat.ResponseDelta, error]) iter.Seq2[pluginmcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
	return func(yield func(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
		var accumulated chat.ResponseAccumulator
		var failure error
		capacity, ok := pluginmcp.StreamCapacity(ctx)
		if !ok || capacity <= 0 {
			yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{}, errors.New("plugin: model stream response capacity is required"))
			return
		}
		var consumed int64
		for delta, err := range source() {
			if err != nil {
				failure = err
				break
			}
			encoded, err := json.Marshal(delta)
			if err != nil || int64(len(encoded)) > capacity-consumed {
				failure = errors.Join(err, errors.New("plugin: model stream aggregate capacity exceeded"))
				break
			}
			consumed += int64(len(encoded))
			if err := accumulated.Add(delta); err != nil {
				failure = err
				break
			}
			if !yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{Chunk: delta.Clone()}, nil) {
				return
			}
		}
		response, err := accumulated.Response()
		if err != nil {
			response = nil
			failure = errors.Join(failure, err)
		}
		result, err := modelResult(response, failure)
		if err != nil {
			yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
			return
		}
		yield(pluginmcp.StreamValue[chat.ResponseDelta, ModelResult]{Result: &result}, nil)
	}
}
