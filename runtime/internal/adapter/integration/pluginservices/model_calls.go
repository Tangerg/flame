package pluginservices

import (
	"context"
	"errors"
	"iter"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/scope/core/chat"
)

// ModelCallbacks uses the existing observed provider resolver and retained chain.
// Each request is independent; a continuation still refers to its frozen endpoint.
func (o *CallComposition) ModelCallbacks(resolve models.ChatModelResolver) ([]pluginsdk.Callback, error) {
	if dependency.Missing(resolve) {
		return nil, errors.New("model composition: selected model resolver is required")
	}
	capture := func(ctx context.Context, input pluginsdk.ModelCallRequest) (models.SelectedChat, *chat.Request, func(), error) {
		if err := input.Validate(); err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		invocation, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		if invocation.ID == executionctx.InvocationOwner(ctx) {
			return models.SelectedChat{}, nil, nil, errors.New("model call: independent invocation cannot claim its executing owner identity")
		}

		selection, err := modelchoice.NewWithReasoningEffort(input.Selection.Provider, input.Selection.Model, input.Selection.ReasoningEffort)
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		selection, err = o.SelectModel(ctx, modelapp.SelectionInput{
			Context: modelchoice.SelectionContext{Use: modelchoice.Independent, Limits: &modelchoice.SelectionLimits{
				InputKind: modelchoice.RequestBytes, MaxInputBytes: input.Budget.MaxRequestBytes, MaxOutputTokens: input.Budget.MaxOutputTokens,
			}},
			Proposed: selection,
			Request:  &input.Request,
		})
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		resolved, err := resolve.ResolveChat(ctx, selection)
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		selected, err := models.NewSelectedChat(selection, resolved)
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		captured, release, err := o.capture(ctx, selected, func() {}, input.Budget.Admit)
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		request := input.Request.Clone()
		request.Options.Model = selection.Model()
		request.Options.ReasoningEffort = chat.ReasoningEffort(selection.ReasoningEffort())
		return captured, request, release, nil
	}
	call, err := pluginsdk.NewSelectedModelCallback(func(ctx context.Context, command pluginsdk.Command[pluginsdk.ModelCallRequest]) (*chat.Response, error) {
		selected, request, release, err := capture(ctx, command.Input)
		if err != nil {
			return nil, err
		}
		defer release()
		return selected.Model().Call(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	stream, err := pluginsdk.NewSelectedModelStreamCallback(func(ctx context.Context, command pluginsdk.Command[pluginsdk.ModelCallRequest]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			selected, request, release, err := capture(ctx, command.Input)
			if err != nil {
				yield(nil, err)
				return
			}
			defer release()
			streamer, ok := selected.Model().(chat.Streamer)
			if !ok {
				yield(nil, errors.New("model call: selected provider does not support streaming"))
				return
			}
			streamer.Stream(ctx, request)(yield)
		}
	})
	if err != nil {
		return nil, err
	}
	return []pluginsdk.Callback{call, stream}, nil
}
