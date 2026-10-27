package pluginservices

import (
	"context"
	"errors"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/scope/core/chat"
)

func (o *CallComposition) ResolveInput(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}
func (o *CallComposition) ValidateInput(ctx context.Context, bindings []plugins.ContributionBinding) error {
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return err
	}
	return o.validateCompositionBindings(ctx, contract, bindings)
}
func (o *CallComposition) PrepareInput(ctx context.Context, bindings []plugins.ContributionBinding, selection modelchoice.Selection, request *chat.Request) (*chat.Request, error) {
	if err := selection.ValidateExact(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, errors.New("model input: request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if _, err := qimumcp.InvocationFromContext(ctx); err != nil {
		return nil, err
	}
	if err := o.ValidateInput(ctx, bindings); err != nil {
		return nil, err
	}
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	method, err := pluginsdk.ModelInputMethod()
	if err != nil {
		return nil, err
	}
	current := request.Clone()
	for _, binding := range bindings {
		calling, err := enterContributionPhase(ctx, binding)
		if err != nil {
			return nil, err
		}
		borrowed, err := borrowContribution(calling, o.calls.pool, binding)
		if err != nil {
			return nil, err
		}
		defer borrowed.Close()
		calling = borrowed.Context()
		session := borrowed.Session()
		if _, err := o.calls.pool.SessionActor(calling, binding.ServerID, session); err != nil {
			return nil, err
		}
		exported, err := qimumcp.ResolveCapability(calling, session, binding.Export, contract)
		if err != nil {
			return nil, err
		}
		operation, err := method.Bind(exported)
		if err != nil {
			return nil, err
		}
		metadata, release, err := o.calls.pool.OpenRPC(calling, binding.ServerID, session)
		if err != nil {
			return nil, err
		}
		result, err := func() (chat.Request, error) {
			defer release()
			return operation.CallWithMetadata(calling, pluginsdk.ModelInputRequest{Selection: pluginsdk.ModelSelection{Provider: selection.Provider(), Model: selection.Model(), ReasoningEffort: selection.ReasoningEffort()}, Request: *current.Clone()}, metadata)
		}()
		if err != nil {
			return nil, err
		}
		if err := pluginsdk.ValidateModelInput(current, &result); err != nil {
			return nil, err
		}
		current = result.Clone()
	}
	return current, nil
}
