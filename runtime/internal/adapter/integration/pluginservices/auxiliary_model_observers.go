package pluginservices

import (
	"context"
	"errors"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/scope/core/chat"
)

// Auxiliary freezes one selected model and its contributions for the use case.
// Memory retains this capture across extraction and curation with its own envelope.
func (o *CallComposition) Auxiliary(resolve models.AuxiliaryResolver) models.AuxiliaryResolver {
	return func(ctx context.Context, budget models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		if err := budget.Validate(); err != nil {
			return models.SelectedChat{}, nil, err
		}
		if resolve == nil {
			return models.SelectedChat{}, nil, errors.New("model composition: auxiliary resolver is required")
		}
		resolved, release, err := resolve(ctx, budget)
		if err != nil {
			return models.SelectedChat{}, nil, err
		}
		return o.capture(ctx, resolved, release, func(request *chat.Request) error { return admitAuxiliary(budget, request) })
	}
}

func admitAuxiliary(budget models.AuxiliaryBudget, request *chat.Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens <= 0 || *request.Options.MaxOutputTokens > budget.MaxOutputTokens {
		return errors.New("model composition: auxiliary output exceeds the captured budget")
	}
	if len(request.Tools) > 0 || request.ToolChoice != nil {
		return errors.New("model composition: auxiliary text generation cannot advertise Tools")
	}
	remaining := budget.MaxInputBytes
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.Kind != chat.PartText {
				return errors.New("model composition: auxiliary input must contain text")
			}
			if len(part.Text) > remaining {
				return errors.New("model composition: auxiliary input exceeds the captured budget")
			}
			remaining -= len(part.Text)
		}
	}
	return nil
}
