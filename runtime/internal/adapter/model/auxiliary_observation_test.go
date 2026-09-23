package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestAuxiliaryCallObservation pins what this span owns: the product operation
// that asked for a utility completion, its input budget, and where it stopped.
func TestAuxiliaryCallObservation(t *testing.T) {
	message := chat.NewAssistantMessage(chat.NewTextPart("private response"))
	for _, tc := range []struct {
		name    string
		usage   *chat.Usage
		finish  chat.FinishReason
		failure error
		stage   string
		refusal string
	}{
		{name: "reported usage", usage: &chat.Usage{InputTokens: 12, OutputTokens: 4, CacheReadInputTokens: new(int64(0))}, finish: chat.FinishReasonStop},
		{name: "unreported usage", finish: chat.FinishReasonStop},
		{name: "resolution failure", failure: errors.New("private resolution failure"), stage: "resolve"},
		{name: "deadline", failure: context.DeadlineExceeded, stage: "call"},
		{name: "canceled", failure: fmt.Errorf("private failure: %w", context.Canceled), stage: "call"},
		{name: "incomplete response", usage: &chat.Usage{InputTokens: 12, OutputTokens: 4}, finish: chat.FinishReasonLength, stage: "response"},
		{name: "refusal with text", usage: &chat.Usage{InputTokens: 12, OutputTokens: 4}, finish: chat.FinishReasonStop, refusal: "private refusal", stage: "response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(context.Background()) })
			ctx, parent := provider.Tracer("test").Start(t.Context(), "maintenance")
			defer parent.End()
			outputMessage := message.Clone()
			if tc.refusal != "" {
				outputMessage.Parts = append(outputMessage.Parts, chat.NewRefusalPart(tc.refusal))
			}
			model := auxiliaryResponseModel{
				response: &chat.Response{
					Output:   &chat.Output{Message: &outputMessage, FinishReason: tc.finish},
					Metadata: &chat.ResponseMetadata{Usage: tc.usage},
				},
				failure: tc.failure,
			}
			resolver, err := LiveUtilityModel(recordingChatResolver{
				resolve: func(modelref.Selection) (ResolvedChat, error) {
					if tc.stage == "resolve" {
						return ResolvedChat{}, tc.failure
					}
					return mustResolvedChat(t, model, nil), nil
				},
			}, mustRoleSelection(t, "anthropic", "claude-test"), staticRoleSource{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = resolver.Complete(ctx, AuxiliaryPrompt{
				Operation: "test", SystemPrompt: "private instructions", UserPrompt: "private prompt",
				MaxInputBytes: 1024, MaxOutputTokens: 128,
			})
			if (err != nil) != (tc.stage != "") {
				t.Fatalf("completion error = %v", err)
			}
			if tc.failure != nil && !errors.Is(err, tc.failure) {
				t.Fatalf("failure cause lost: %v", err)
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("completed auxiliary spans = %d, want 1", len(spans))
			}
			span := spans[0]
			if span.Name != "auxiliary model" || !span.Parent.Equal(parent.SpanContext()) {
				t.Fatalf("incorrect attribution: %s %v", span.Name, span.Parent)
			}
			if span.EndTime.Before(span.StartTime) {
				t.Fatal("invalid call duration")
			}
			attrs := map[string]attribute.Value{}
			for _, a := range span.Attributes {
				attrs[string(a.Key)] = a.Value
				if strings.Contains(a.Value.Emit(), "private") {
					t.Fatalf("content exposed in %s", a.Key)
				}
			}
			if len(span.Events) != 0 {
				t.Fatal("raw failure or content recorded")
			}
			if attrs["auxiliary.operation"].AsString() != "test" {
				t.Fatal("operation absent")
			}
			if attrs["flame.provider.id"].AsString() != "anthropic" || attrs["gen_ai.request.model"].AsString() != "claude-test" {
				t.Fatal("resolved selection absent")
			}
			if errors.Is(tc.failure, context.Canceled) && attrs["error.type"].AsString() != "canceled" {
				t.Fatal("cancellation category absent")
			}
			if errors.Is(tc.failure, context.DeadlineExceeded) && attrs["error.type"].AsString() != "deadline_exceeded" {
				t.Fatal("deadline category absent")
			}
			if tc.stage != "" && (span.Status.Code != codes.Error || attrs["auxiliary.failure_stage"].AsString() != tc.stage) {
				t.Fatalf("failure observation = %+v", span)
			}
			// Request, usage, and finish-reason facts have one producer: the
			// Scope middleware wrapping the provider. A copy here would be a
			// second series for the same call.
			for key := range attrs {
				if strings.HasPrefix(key, "gen_ai.usage.") || key == "gen_ai.response.finish_reason" ||
					key == "gen_ai.request.max_tokens" {
					t.Fatalf("auxiliary span restates model telemetry: %s", key)
				}
			}
		})
	}
}
