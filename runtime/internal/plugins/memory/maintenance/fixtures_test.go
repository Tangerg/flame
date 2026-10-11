package maintenance

import (
	"context"

	model "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	modeltext "github.com/Tangerg/flame/runtime/internal/adapter/run/modeltext"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	"github.com/Tangerg/scope/core/chat"
)

func intPointer(value int) *int { return &value }

type capturedHistory struct {
	source interface {
		Read(context.Context, string) ([]chat.Message, error)
	}
}

func (h capturedHistory) Read(ctx context.Context, id string) (Conversation, error) {
	messages, err := h.source.Read(ctx, id)
	return Conversation{len(messages), modeltext.RenderTranscript(messages)}, err
}

func capturedCompletion(client chat.Model) managedmcp.Completion {
	resolver := model.AuxiliaryResolver(func(context.Context) (chat.Model, error) { return client, nil })
	return func(ctx context.Context, prompt managedmcp.Prompt) (string, error) {
		return resolver.Complete(ctx, model.AuxiliaryPrompt{Operation: prompt.Operation, SystemPrompt: prompt.SystemPrompt, UserPrompt: prompt.UserPrompt, MaxInputBytes: prompt.MaxInputBytes, MaxOutputTokens: prompt.MaxOutputTokens})
	}
}
