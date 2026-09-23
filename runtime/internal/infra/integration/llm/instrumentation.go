package llm

import (
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	otelchat "github.com/Tangerg/scope/otel/chat"
)

// instrumentModel wraps one provider instance in Scope's GenAI telemetry, which
// owns the semantic conventions for model spans, token counters, and streaming
// latency. It sits closest to the provider so a span records the provider's own
// failure and timing, before Runtime translates either.
//
// Token counting is a provider request the model never answers with generated
// content, so it stays outside the GenAI operation telemetry and is republished
// unchanged.
func instrumentModel(provider Provider, model chat.Model) (chat.Model, error) {
	middleware, err := otelchat.NewMiddleware(otelchat.MiddlewareConfig{Provider: string(provider)})
	if err != nil {
		return nil, fmt.Errorf("llm: instrument %s model: %w", provider, err)
	}
	return decorateModel(model, modelDecoration{
		name:   "instrument " + string(provider) + " model",
		call:   middleware.Call,
		stream: middleware.Stream,
	})
}
