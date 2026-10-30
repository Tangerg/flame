package pluginservices

import (
	"context"
	"errors"
	"iter"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// ModelCallbacks uses the existing observed provider resolver and retained chain.
// Each request is independent; a continuation still refers to its frozen endpoint.
func (o *CallComposition) ModelCallbacks(resolve models.ChatModelResolver) ([]qimumcp.Callback, error) {
	if lo.IsNil(resolve) {
		return nil, errors.New("model composition: selected model resolver is required")
	}
	capture := func(ctx context.Context, input pluginsdk.ModelCallRequest) (models.SelectedChat, *chat.Request, func() error, error) {
		if err := input.Validate(); err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		invocation, err := qimumcp.InvocationFromContext(ctx)
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
			return models.SelectedChat{}, nil, nil, errors.Join(err, resolved.Close(context.WithoutCancel(ctx)))
		}
		captured, release, err := o.capture(ctx, selected, func() error { return resolved.Close(context.WithoutCancel(ctx)) }, input.Budget.Admit)
		if err != nil {
			return models.SelectedChat{}, nil, nil, err
		}
		request := input.Request.Clone()
		request.Options.Model = selection.Model()
		request.Options.ReasoningEffort = chat.ReasoningEffort(selection.ReasoningEffort())
		return captured, request, release, nil
	}
	call, err := pluginsdk.NewSelectedModelCallback(func(ctx context.Context, command qimumcp.Command[pluginsdk.ModelCallRequest]) (response *chat.Response, returned error) {
		selected, request, release, err := capture(ctx, command.Input)
		if err != nil {
			return nil, err
		}
		defer func() { returned = errors.Join(returned, release()) }()
		return selected.Model().Call(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	stream, err := pluginsdk.NewSelectedModelStreamCallback(func(ctx context.Context, command qimumcp.Command[pluginsdk.ModelCallRequest]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			selected, request, release, err := capture(ctx, command.Input)
			if err != nil {
				yield(nil, err)
				return
			}
			continuing := true
			defer func() {
				if err := release(); err != nil && continuing {
					yield(nil, err)
				}
			}()
			streamer, ok := selected.Model().(chat.Streamer)
			if !ok {
				yield(nil, errors.New("model call: selected provider does not support streaming"))
				return
			}
			streamer.Stream(ctx, request)(func(delta *chat.ResponseDelta, err error) bool { continuing = yield(delta, err); return continuing })
		}
	})
	if err != nil {
		return nil, err
	}
	return []qimumcp.Callback{call, stream}, nil
}
