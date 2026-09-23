package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// AuxiliaryResolver selects the current utility-role model for each call.
// Resolving at the boundary lets a role configuration change take effect without
// rebuilding the owning worker.
type AuxiliaryResolver func(context.Context) (chat.Model, error)

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
	// The span carries what Scope cannot see: which product operation asked and
	// how much input it spent. Request, usage, and finish-reason telemetry
	// belongs to the model span Scope's middleware opens at the provider
	// boundary, so this one does not record those facts a second time.
	ctx, span := otel.Tracer("scope/flame/adapter/model").Start(ctx, "auxiliary model",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("auxiliary.operation", prompt.Operation),
			attribute.Int("auxiliary.input_bytes", len(prompt.SystemPrompt)+len(prompt.UserPrompt)),
		),
	)
	stage := "resolve"
	defer func() {
		if err != nil {
			// An unusable completion is the model answering, not the call
			// failing: Scope's output contract is what separates them.
			if stage == "call" && errors.Is(err, chatclient.ErrInvalidOutput) {
				stage = "response"
			}
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
	model, err := r(ctx)
	if err != nil {
		return "", err
	}
	if dependency.Missing(model) {
		return "", errors.New("auxiliary model: model is required")
	}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		return "", err
	}
	stage = "call"
	text, err = client.Output(ctx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(prompt.SystemPrompt),
		chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt)),
	}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens}}, chatclient.Text())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: auxiliary model returned blank text", chatclient.ErrInvalidOutput)
	}
	return text, nil
}
