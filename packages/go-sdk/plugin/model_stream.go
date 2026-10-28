package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"iter"

	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
)

func ModelStreamMethod() (*qimumcp.StreamingMethod[chat.Request, chat.ResponseDelta, ModelResult], error) {
	return qimumcp.NewStreamingMethod[chat.Request, chat.ResponseDelta, ModelResult](qimumcp.MethodConfig{
		Name: "flame.model/stream.v1", Kind: portable.Mutation, Target: "call",
		Description: "Consume the captured model's canonical stream and final outcome.",
	})
}

func StreamModel(ctx context.Context, ref qimumcp.CallbackRef, id string, request *chat.Request) iter.Seq2[qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
	method, err := ModelStreamMethod()
	if err == nil {
		err = request.Validate()
	}
	if err != nil {
		return func(yield func(qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
		}
	}
	return qimumcp.StreamHost(ctx, ref, method, id, *request.Clone())
}

func NewModelStreamCallback(stream func(context.Context, qimumcp.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error]) (qimumcp.Callback, error) {
	if stream == nil {
		return qimumcp.Callback{}, errors.New("plugin: model stream owner is required")
	}
	method, err := ModelStreamMethod()
	if err != nil {
		return qimumcp.Callback{}, err
	}
	return qimumcp.NewStreamingCallback(method, func(ctx context.Context, command qimumcp.Command[chat.Request]) iter.Seq2[qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
		return func(yield func(qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
			if err := command.Input.Validate(); err != nil {
				yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			command.Input = *command.Input.Clone()
			ctx, err := modelCallContext(ctx, command.ID)
			if err != nil {
				yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
				return
			}
			modelStreamResult(ctx, func() iter.Seq2[*chat.ResponseDelta, error] { return stream(ctx, command) })(yield)
		}
	})
}

func modelStreamResult(ctx context.Context, source func() iter.Seq2[*chat.ResponseDelta, error]) iter.Seq2[qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error] {
	return func(yield func(qimumcp.StreamValue[chat.ResponseDelta, ModelResult], error) bool) {
		var accumulated chat.ResponseAccumulator
		var failure error
		capacity, ok := qimumcp.StreamCapacity(ctx)
		if !ok || capacity <= 0 {
			yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{}, errors.New("plugin: model stream response capacity is required"))
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
			if !yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{Chunk: delta.Clone()}, nil) {
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
			yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{}, err)
			return
		}
		yield(qimumcp.StreamValue[chat.ResponseDelta, ModelResult]{Result: &result}, nil)
	}
}
