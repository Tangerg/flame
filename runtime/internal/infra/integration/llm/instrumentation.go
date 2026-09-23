package llm

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/scope/core/chat"
	otelchat "github.com/Tangerg/scope/otel/chat"
)

// instrumentModel wraps one provider instance in Scope's GenAI telemetry, which
// owns the semantic conventions for model spans, token counters, and streaming
// latency. It sits closest to the provider so a span records the provider's own
// failure and timing, before Runtime translates either.
//
// A model publishes streaming and token counting as optional capabilities of
// the same instance, so the wrapper republishes exactly the ones it received.
func instrumentModel(provider Provider, model chat.Model) (chat.Model, error) {
	middleware, err := otelchat.NewMiddleware(otelchat.MiddlewareConfig{Provider: string(provider)})
	if err != nil {
		return nil, fmt.Errorf("llm: instrument %s model: %w", provider, err)
	}
	instrumented := &instrumentedModel{model: middleware.Call(model)}
	if instrumented.model == nil {
		return nil, fmt.Errorf("llm: instrument %s model: middleware returned no model", provider)
	}
	streamer, streams := model.(chat.Streamer)
	counter, counts := model.(InputTokenCounter)
	switch {
	case streams && counts:
		return &instrumentedStreamingCountingModel{
			instrumentedCountingModel: instrumentedCountingModel{instrumentedModel: instrumented, counter: counter},
			streamer:                  middleware.Stream(streamer),
		}, nil
	case streams:
		return &instrumentedStreamingModel{instrumentedModel: instrumented, streamer: middleware.Stream(streamer)}, nil
	case counts:
		return &instrumentedCountingModel{instrumentedModel: instrumented, counter: counter}, nil
	default:
		return instrumented, nil
	}
}

type instrumentedModel struct {
	model chat.Model
}

func (i *instrumentedModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	return i.model.Call(ctx, request)
}

type instrumentedStreamingModel struct {
	*instrumentedModel
	streamer chat.Streamer
}

func (i *instrumentedStreamingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return i.streamer.Stream(ctx, request)
}

// Token counting is a provider request the model never answers with generated
// content, so it stays outside the GenAI operation telemetry.
type instrumentedCountingModel struct {
	*instrumentedModel
	counter InputTokenCounter
}

func (i *instrumentedCountingModel) CountInputTokens(ctx context.Context, request *chat.Request) (int64, error) {
	return i.counter.CountInputTokens(ctx, request)
}

type instrumentedStreamingCountingModel struct {
	instrumentedCountingModel
	streamer chat.Streamer
}

func (i *instrumentedStreamingCountingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return i.streamer.Stream(ctx, request)
}
