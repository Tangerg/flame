package llm

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/scope/core/chat"
)

// modelDecoration replaces one provider instance's behaviour. Each field
// transforms the capability it names; a nil field publishes that capability
// unchanged.
type modelDecoration struct {
	name   string
	call   func(chat.Model) chat.Model
	stream func(chat.Streamer) chat.Streamer
	count  func(InputTokenCounter) InputTokenCounter
}

// decorateModel republishes exactly the optional capabilities the inner
// instance had. A provider publishes streaming and complete-request token
// counting as assertions on the same value, so a wrapper that returns a plain
// chat.Model silently downgrades the model instead of decorating it — which is
// why every wrapper in this package assembles its result here rather than
// writing the four combinations again.
func decorateModel(inner chat.Model, decoration modelDecoration) (chat.Model, error) {
	if inner == nil {
		return nil, fmt.Errorf("llm: %s: model is nil", decoration.name)
	}
	decorated := decoration.call(inner)
	if decorated == nil {
		return nil, fmt.Errorf("llm: %s: decoration returned no model", decoration.name)
	}
	base := decoratedModel{model: decorated}
	streamer, streams := inner.(chat.Streamer)
	counter, counts := inner.(InputTokenCounter)
	if streams && decoration.stream != nil {
		streamer = decoration.stream(streamer)
		if streamer == nil {
			return nil, fmt.Errorf("llm: %s: decoration returned no streamer", decoration.name)
		}
	}
	if counts && decoration.count != nil {
		counter = decoration.count(counter)
		if counter == nil {
			return nil, fmt.Errorf("llm: %s: decoration returned no token counter", decoration.name)
		}
	}
	switch {
	case streams && counts:
		return &decoratedStreamingCountingModel{
			decoratedCountingModel: decoratedCountingModel{decoratedModel: base, counter: counter},
			streamer:               streamer,
		}, nil
	case streams:
		return &decoratedStreamingModel{decoratedModel: base, streamer: streamer}, nil
	case counts:
		return &decoratedCountingModel{decoratedModel: base, counter: counter}, nil
	default:
		return &base, nil
	}
}

type decoratedModel struct {
	model chat.Model
}

func (d *decoratedModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	return d.model.Call(ctx, request)
}

type decoratedStreamingModel struct {
	decoratedModel
	streamer chat.Streamer
}

func (d *decoratedStreamingModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return d.streamer.Stream(ctx, request)
}

type decoratedCountingModel struct {
	decoratedModel
	counter InputTokenCounter
}

func (d *decoratedCountingModel) CountInputTokens(
	ctx context.Context,
	request *chat.Request,
) (int64, error) {
	return d.counter.CountInputTokens(ctx, request)
}

type decoratedStreamingCountingModel struct {
	decoratedCountingModel
	streamer chat.Streamer
}

func (d *decoratedStreamingCountingModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return d.streamer.Stream(ctx, request)
}
