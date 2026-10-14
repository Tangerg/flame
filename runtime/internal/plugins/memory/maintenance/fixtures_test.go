package maintenance

import (
	"context"

	model "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	modeltext "github.com/Tangerg/flame/runtime/internal/adapter/run/modeltext"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
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

type capturedUtility struct{ resolver model.AuxiliaryResolver }

func capturedCompletion(client chat.Model) capturedUtility {
	resolver := model.AuxiliaryResolver(func(context.Context, model.AuxiliaryBudget) (model.AuxiliaryModel, func(), error) {
		return auxiliaryFixture(client), func() {}, nil
	})
	return capturedUtility{resolver: resolver}
}

func (c capturedUtility) Complete(ctx context.Context, prompt rpc.Prompt) (string, error) {
	return c.resolver.Complete(ctx, model.AuxiliaryPrompt{Operation: prompt.Operation, SystemPrompt: prompt.SystemPrompt, UserPrompt: prompt.UserPrompt, MaxInputBytes: prompt.MaxInputBytes, MaxOutputTokens: prompt.MaxOutputTokens})
}
