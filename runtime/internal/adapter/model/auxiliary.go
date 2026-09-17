package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// AuxiliaryResolver selects the current utility-role client for each call.
// Resolving at the boundary lets a role configuration change take effect without
// rebuilding the owning worker.
type AuxiliaryResolver func(context.Context) (*chatclient.Client, error)

// AuxiliaryPrompt is the complete resource envelope for one auxiliary model request.
// Input bytes and output tokens are deliberately mandatory: background
// maintenance must never inherit a provider's context/output defaults.
type AuxiliaryPrompt struct {
	Operation       string
	SystemPrompt    string
	UserPrompt      string
	MaxInputBytes   int
	MaxOutputTokens int64
}

func (p AuxiliaryPrompt) validate() error {
	if strings.TrimSpace(p.Operation) == "" {
		return errors.New("auxiliary model: operation is required")
	}
	if p.MaxInputBytes <= 0 {
		return errors.New("auxiliary model: max input bytes must be positive")
	}
	if p.MaxOutputTokens <= 0 {
		return errors.New("auxiliary model: max output tokens must be positive")
	}
	inputBytes := len(p.SystemPrompt) + len(p.UserPrompt)
	if inputBytes > p.MaxInputBytes {
		return fmt.Errorf(
			"auxiliary model: prompt is %d bytes; input limit is %d",
			inputBytes,
			p.MaxInputBytes,
		)
	}
	return nil
}

// Complete resolves the live auxiliary selection and returns a complete text
// generation inside the caller's resource envelope. A rejected or incomplete
// generation cannot become a durable summary, memory, or skill proposal.
func (r AuxiliaryResolver) Complete(ctx context.Context, prompt AuxiliaryPrompt) (text string, err error) {
	if r == nil {
		return "", errors.New("auxiliary model: resolver is required")
	}
	if err := prompt.validate(); err != nil {
		return "", err
	}
	ctx, span := otel.Tracer("scope/flame/adapter/model").Start(ctx, "auxiliary model",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("auxiliary.operation", prompt.Operation),
			attribute.Int("auxiliary.input_bytes", len(prompt.SystemPrompt)+len(prompt.UserPrompt)),
			attribute.Int64("gen_ai.request.max_tokens", prompt.MaxOutputTokens),
		),
	)
	stage := "resolve"
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, "auxiliary model failed")
			span.SetAttributes(attribute.String("auxiliary.failure_stage", stage))
			if errors.Is(err, context.Canceled) {
				span.SetAttributes(attribute.String("error.type", "canceled"))
			} else if errors.Is(err, context.DeadlineExceeded) {
				span.SetAttributes(attribute.String("error.type", "deadline_exceeded"))
			}
		}
		span.End()
	}()
	client, err := r(ctx)
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", errors.New("auxiliary model: client is required")
	}
	stage = "call"
	response, err := client.Call(ctx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(prompt.SystemPrompt),
		chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt)),
	}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens}})
	if err != nil {
		return "", err
	}
	stage = "response"
	if err := response.Validate(); err != nil {
		return "", fmt.Errorf("auxiliary model: invalid response: %w", err)
	}
	span.SetAttributes(attribute.String("gen_ai.response.finish_reason", string(response.Output.FinishReason)))
	if response.Metadata != nil && response.Metadata.Usage != nil {
		usage := response.Metadata.Usage
		span.SetAttributes(attribute.Int64("gen_ai.usage.input_tokens", usage.InputTokens), attribute.Int64("gen_ai.usage.output_tokens", usage.OutputTokens))
		if usage.CacheReadInputTokens != nil {
			span.SetAttributes(attribute.Int64("gen_ai.usage.cache_read.input_tokens", *usage.CacheReadInputTokens))
		}
		if usage.CacheWriteInputTokens != nil {
			span.SetAttributes(attribute.Int64("gen_ai.usage.cache_write.input_tokens", *usage.CacheWriteInputTokens))
		}
		if usage.ReasoningTokens != nil {
			span.SetAttributes(attribute.Int64("gen_ai.usage.reasoning_tokens", *usage.ReasoningTokens))
		}
	}
	if response.Output.FinishReason != chat.FinishReasonStop {
		return "", fmt.Errorf("auxiliary model: generation did not complete (finish reason %q)", response.Output.FinishReason)
	}
	text = response.Text()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf(
			"auxiliary model: completed without text (finish reason %q)",
			response.Output.FinishReason,
		)
	}
	return text, nil
}
