package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/scope/core/chat"
)

func ModelStreamMethod() (*StreamingMethod[chat.Request, chat.ResponseDelta, ModelResult], error) {
	return NewStreamingMethod[chat.Request, chat.ResponseDelta, ModelResult](MethodConfig{
		Name: "flame.model/stream.v1", Kind: capability.Mutation, Target: "call",
		Description: "Consume the captured model's canonical stream and final outcome.",
	})
}

func StreamModel(ctx context.Context, ref CallbackRef, id string, request *chat.Request) iter.Seq2[StreamValue[chat.ResponseDelta, ModelResult], error] {
	method, err := ModelStreamMethod()
	if err == nil {
		err = request.Validate()
	}
	if err != nil {
		return func(yield func(StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			yield(StreamValue[chat.ResponseDelta, ModelResult]{}, err)
		}
	}
	return StreamHost(ctx, ref, method, id, *request.Clone())
}

func NewModelStreamCallback(stream func(context.Context, Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error]) (Callback, error) {
	if stream == nil {
		return Callback{}, errors.New("plugin: model stream owner is required")
	}
	method, err := ModelStreamMethod()
	if err != nil {
		return Callback{}, err
	}
	return NewStreamingCallback(method, func(ctx context.Context, command Command[chat.Request]) iter.Seq2[StreamValue[chat.ResponseDelta, ModelResult], error] {
		return func(yield func(StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			if err := command.Input.Validate(); err != nil {
				yield(StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			command.Input = *command.Input.Clone()
			ctx, err := modelCallContext(ctx, command.ID)
			if err != nil {
				yield(StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			modelStreamResult(ctx, func() iter.Seq2[*chat.ResponseDelta, error] { return stream(ctx, command) })(yield)
		}
	})
}

func modelStreamResult(ctx context.Context, source func() iter.Seq2[*chat.ResponseDelta, error]) iter.Seq2[StreamValue[chat.ResponseDelta, ModelResult], error] {
	return func(yield func(StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
		var accumulated chat.ResponseAccumulator
		var failure error
		capacity, ok := ctx.Value(streamCapacityKey{}).(int64)
		if !ok || capacity <= 0 {
			yield(StreamValue[chat.ResponseDelta, ModelResult]{}, errors.New("plugin: model stream response capacity is required"))
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
			if !yield(StreamValue[chat.ResponseDelta, ModelResult]{Chunk: delta.Clone()}, nil) {
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
			yield(StreamValue[chat.ResponseDelta, ModelResult]{}, err)
			return
		}
		yield(StreamValue[chat.ResponseDelta, ModelResult]{Result: &result}, nil)
	}
}
